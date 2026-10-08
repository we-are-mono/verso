// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! A signed-in session given what signing in now would give it (ADR-007 §8).
//! rpcd fills a session's grants once, at login, from the access lists on disk
//! then; a plugin installed afterwards ships a group the session never got.
//! This derives a session's grants the way rpcd's login does (`session.c`,
//! `rpc_login_setup_acls`) and adds them with `session.grant`. It only ever
//! adds what a login now would, and rpcd still decides every call.
use crate::Failure;
use serde_json::{json, Map, Value};
use std::collections::{BTreeMap, BTreeSet};
use std::ffi::CString;
use std::fs;
use std::os::raw::{c_char, c_int};
use std::process::{Command, Stdio};

const ACL_DIR: &str = "/usr/share/rpcd/acl.d";

extern "C" {
    /// The C library's own, which rpcd matches group names with.
    fn fnmatch(pattern: *const c_char, string: *const c_char, flags: c_int) -> c_int;
}

/// A grant: scope, object, function.
type Grant = (String, String, String);

/// Login is the `read` and `write` lists of a `config login`; an option given
/// as a single value rather than a list counts for nothing, as in rpcd.
#[derive(Default, Debug, PartialEq)]
struct Login {
    read: Vec<String>,
    write: Vec<String>,
}

pub fn refresh(sid: &str) -> Result<Value, Failure> {
    let session = ubus("session", "list", json!({ "ubus_rpc_session": sid }))?;
    let user = session["data"]["username"]
        .as_str()
        .filter(|u| !u.is_empty())
        .ok_or_else(|| Failure::invalid("The session names no user."))?;
    let logins = ubus("uci", "get", json!({ "config": "rpcd", "type": "login" }))?;
    let Some(login) = login_for(&logins, user) else {
        return Ok(json!({ "granted": 0 }));
    };
    let mut paths: Vec<_> = fs::read_dir(ACL_DIR)
        .into_iter()
        .flatten()
        .flatten()
        .map(|e| e.path())
        .filter(|p| p.extension().is_some_and(|x| x == "json"))
        .collect();
    paths.sort();
    let files: Vec<Value> = paths
        .iter()
        .filter_map(|p| serde_json::from_str(&fs::read_to_string(p).ok()?).ok())
        .collect();
    let grants = acl_grants(&files, &login);
    let mut by_scope: BTreeMap<&str, Vec<Value>> = BTreeMap::new();
    for (scope, object, function) in &grants {
        by_scope
            .entry(scope)
            .or_default()
            .push(json!([object, function]));
    }
    for (scope, objects) in by_scope {
        ubus(
            "session",
            "grant",
            json!({ "ubus_rpc_session": sid, "scope": scope, "objects": objects }),
        )?;
    }
    Ok(json!({ "granted": grants.len() }))
}

fn ubus(object: &str, method: &str, args: Value) -> Result<Value, Failure> {
    let out = Command::new("/bin/ubus")
        .args(["call", object, method, &args.to_string()])
        .stdin(Stdio::null())
        .output()
        .map_err(|e| Failure::unknown(format!("ubus: {e}")))?;
    if !out.status.success() {
        return Err(Failure::unknown(format!(
            "ubus call {object} {method} failed"
        )));
    }
    let text = String::from_utf8_lossy(&out.stdout);
    match text.trim() {
        "" => Ok(Value::Null),
        t => serde_json::from_str(t).map_err(|e| Failure::unknown(format!("ubus: {e}"))),
    }
}

/// login_for is the first `config login` naming the user, as rpcd looks it up.
fn login_for(logins: &Value, user: &str) -> Option<Login> {
    let mut sections: Vec<&Value> = logins["values"]
        .as_object()?
        .values()
        .filter(|s| s[".type"] == "login")
        .collect();
    sections.sort_by_key(|s| s[".index"].as_u64().unwrap_or(u64::MAX));
    let section = sections.into_iter().find(|s| s["username"] == user)?;
    let list = |key: &str| -> Vec<String> {
        section[key]
            .as_array()
            .map(|a| {
                a.iter()
                    .filter_map(Value::as_str)
                    .map(String::from)
                    .collect()
            })
            .unwrap_or_default()
    };
    Some(Login {
        read: list("read"),
        write: list("write"),
    })
}

fn matches(pattern: &str, name: &str) -> bool {
    let (Ok(p), Ok(n)) = (CString::new(pattern), CString::new(name)) else {
        return false;
    };
    // SAFETY: both are NUL-terminated strings that outlive the call.
    unsafe { fnmatch(p.as_ptr(), n.as_ptr(), 0) == 0 }
}

/// permitted is rpcd's `rpc_login_test_permission`: a `!` pattern that matches
/// denies outright; otherwise any other pattern that matches allows; and
/// `write` implies `read`.
fn permitted(login: &Login, perm: &str, group: &str) -> bool {
    let list = if perm == "read" {
        &login.read
    } else {
        &login.write
    };
    let denied = list
        .iter()
        .filter_map(|p| p.strip_prefix('!'))
        .map(str::trim_start)
        .any(|p| !p.is_empty() && matches(p, group));
    if denied {
        return false;
    }
    if list
        .iter()
        .any(|p| !p.is_empty() && !p.starts_with('!') && matches(p, group))
    {
        return true;
    }
    perm == "read" && permitted(login, "write", group)
}

/// acl_grants is every grant rpcd's login gives this login from these access
/// list files: for each group's `read` and `write` the login allows, a table
/// scope grants each object's functions, a list scope grants each object with
/// the permission as its function, and the group itself joins `access-group`.
fn acl_grants(files: &[Value], login: &Login) -> BTreeSet<Grant> {
    let mut out = BTreeSet::new();
    let empty = Map::new();
    for (group, perms) in files.iter().flat_map(|f| f.as_object().unwrap_or(&empty)) {
        for (perm, scopes) in perms.as_object().unwrap_or(&empty) {
            let Some(scopes) = scopes.as_object() else {
                continue;
            };
            if !matches!(perm.as_str(), "read" | "write") || !permitted(login, perm, group) {
                continue;
            }
            for (scope, entries) in scopes {
                let mut add = |object: &str, function: &str| {
                    out.insert((scope.clone(), object.to_string(), function.to_string()));
                };
                match entries {
                    Value::Object(objects) => {
                        for (object, functions) in objects {
                            for f in functions.as_array().into_iter().flatten() {
                                if let Some(f) = f.as_str() {
                                    add(object, f);
                                }
                            }
                        }
                    }
                    Value::Array(objects) => {
                        for o in objects.iter().filter_map(Value::as_str) {
                            add(o, perm);
                        }
                    }
                    _ => {}
                }
                out.insert(("access-group".into(), group.clone(), perm.clone()));
            }
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn login(read: &[&str], write: &[&str]) -> Login {
        Login {
            read: read.iter().map(|s| s.to_string()).collect(),
            write: write.iter().map(|s| s.to_string()).collect(),
        }
    }

    fn g(scope: &str, object: &str, function: &str) -> Grant {
        (scope.into(), object.into(), function.into())
    }

    fn ddns() -> Value {
        json!({ "verso-plugin-ddns": {
            "description": "ignored",
            "read": { "uci": ["ddns", "network"], "ubus": { "verso": ["ddnsState"] } },
            "write": { "uci": ["ddns"], "ubus": { "verso": ["ddnsUpdate"] } }
        }})
    }

    fn luci() -> Value {
        json!({ "luci-base": { "read": { "ubus": { "luci": ["getVersion"] } } } })
    }

    #[test]
    fn root_gets_every_group_in_both_notations() {
        let got = acl_grants(&[ddns(), luci()], &login(&["*"], &["*"]));
        for want in [
            g("uci", "ddns", "read"),
            g("uci", "network", "read"),
            g("ubus", "verso", "ddnsState"),
            g("uci", "ddns", "write"),
            g("ubus", "verso", "ddnsUpdate"),
            g("access-group", "verso-plugin-ddns", "read"),
            g("access-group", "verso-plugin-ddns", "write"),
            g("ubus", "luci", "getVersion"),
            g("access-group", "luci-base", "read"),
        ] {
            assert!(got.contains(&want), "missing {want:?}");
        }
        assert_eq!(got.len(), 9);
    }

    #[test]
    fn a_limited_user_gets_only_the_groups_its_lists_name() {
        let got = acl_grants(&[ddns(), luci()], &login(&["luci-*"], &[]));
        assert_eq!(
            got,
            BTreeSet::from([
                g("ubus", "luci", "getVersion"),
                g("access-group", "luci-base", "read")
            ])
        );
    }

    #[test]
    fn write_implies_read_but_a_denied_read_stays_denied() {
        let both = acl_grants(&[ddns()], &login(&[], &["verso-plugin-ddns"]));
        assert!(both.contains(&g("uci", "ddns", "read")));
        assert!(both.contains(&g("uci", "ddns", "write")));
        let write_only = acl_grants(
            &[ddns()],
            &login(&["! verso-plugin-*"], &["verso-plugin-ddns"]),
        );
        assert!(!write_only.contains(&g("uci", "ddns", "read")));
        assert!(write_only.contains(&g("uci", "ddns", "write")));
    }

    #[test]
    fn a_denial_outranks_a_wildcard() {
        let got = acl_grants(&[ddns(), luci()], &login(&["*", "!verso-plugin-*"], &[]));
        assert!(
            got.iter().all(|(_, o, _)| o != "ddns" && o != "verso"),
            "{got:?}"
        );
        assert!(got.contains(&g("ubus", "luci", "getVersion")));
    }

    #[test]
    fn what_rpcd_skips_is_skipped() {
        let odd = json!({
            "a-string": "not a group",
            "odd": { "read": "not a table", "admin": { "uci": ["x"] },
                     "write": { "uci": { "y": "not a list", "z": [1, "set"] } } }
        });
        let got = acl_grants(&[odd, json!(["not an object"])], &login(&["*"], &["*"]));
        assert_eq!(
            got,
            BTreeSet::from([g("uci", "z", "set"), g("access-group", "odd", "write")])
        );
    }

    #[test]
    fn the_first_login_naming_the_user_counts_and_only_its_lists() {
        let logins = json!({ "values": {
            "b": { ".type": "login", ".index": 2, "username": "ops", "read": ["*"] },
            "a": { ".type": "login", ".index": 1, "username": "ops", "read": "luci-*", "write": ["x"] },
            "c": { ".type": "other", ".index": 0, "username": "ops", "read": ["*"] }
        }});
        assert_eq!(login_for(&logins, "ops"), Some(login(&[], &["x"])));
        assert_eq!(login_for(&logins, "nobody"), None);
    }

    #[test]
    fn group_names_match_as_fnmatch_does() {
        assert!(matches("verso-plugin-*", "verso-plugin-ddns"));
        assert!(matches("verso-plugin-dd?s", "verso-plugin-ddns"));
        assert!(matches("luci-[ab]*", "luci-base"));
        assert!(!matches("luci-[!ab]*", "luci-base"));
        assert!(!matches("verso", "verso-plugin-ddns"));
    }
}
