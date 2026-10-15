// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! Fixed-purpose credential files. Public material is readable; private keys
//! only travel into the helper and are never returned.
use crate::Failure;
use serde_json::{json, Value};
use std::fs::{self, OpenOptions};
use std::io::{Read, Write};
use std::os::unix::fs::OpenOptionsExt;
use std::path::{Path, PathBuf};
use std::process::Command;
const KEYS: &str = "/etc/dropbear/authorized_keys";
const LIMIT: usize = 64 * 1024;
/// The certificate Verso serves HTTPS with (ADR-017 §3): the pair's directory.
/// The shell's account alone may read the key; plugins run in its group.
const TLS_DIR: &str = "/etc/verso";
fn tls_files(dir: &Path) -> (PathBuf, PathBuf) {
    (dir.join("tls.crt"), dir.join("tls.key"))
}
fn read(path: &Path) -> Result<String, Failure> {
    let mut file = match fs::File::open(path) {
        Ok(f) => f,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(String::new()),
        Err(e) => return Err(Failure::unknown(e.to_string())),
    };
    let mut s = String::new();
    std::io::Read::by_ref(&mut file)
        .take((LIMIT + 1) as u64)
        .read_to_string(&mut s)
        .map_err(|e| Failure::unknown(e.to_string()))?;
    if s.len() > LIMIT {
        return Err(Failure::invalid("credential file too large"));
    }
    Ok(s)
}
fn read_bytes(path: &Path) -> Result<Vec<u8>, Failure> {
    let file = match fs::File::open(path) {
        Ok(f) => f,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(vec![]),
        Err(e) => return Err(Failure::unknown(e.to_string())),
    };
    let mut bytes = vec![];
    file.take((LIMIT + 1) as u64)
        .read_to_end(&mut bytes)
        .map_err(|e| Failure::unknown(e.to_string()))?;
    if bytes.len() > LIMIT {
        return Err(Failure::invalid("credential file too large"));
    }
    Ok(bytes)
}
pub fn state() -> Result<Value, Failure> {
    let keys = read(Path::new(KEYS))?;
    let (cert, _) = tls_files(Path::new(TLS_DIR));
    let pem = read_bytes(&cert)?;
    Ok(
        json!({"authorized_keys":keys,"certificate_bytes":pem,"certificate_file":cert.to_string_lossy()}),
    )
}
// write_new writes value beside path under a private name, with mode and, when
// given, owner (uid, gid) set before a byte is written: the file is never
// readable by anyone it is not meant for, even before it is renamed into place.
fn write_new(path: &Path, value: &[u8], mode: u32, owner: Option<(u32, u32)>) -> Result<PathBuf, Failure> {
    let mut filename = path
        .file_name()
        .ok_or_else(|| Failure::invalid("invalid destination"))?
        .to_os_string();
    filename.push(format!(".verso-{}", std::process::id()));
    let tmp = path.with_file_name(filename);
    let mut f = OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(mode & 0o600)
        .open(&tmp)
        .map_err(|e| Failure::unknown(e.to_string()))?;
    let written = owner
        .map_or(Ok(()), |(uid, gid)| std::os::unix::fs::fchown(&f, Some(uid), Some(gid)))
        .and_then(|_| f.set_permissions(std::os::unix::fs::PermissionsExt::from_mode(mode)))
        .and_then(|_| f.write_all(value))
        .and_then(|_| f.sync_all());
    if let Err(e) = written {
        let _ = fs::remove_file(&tmp);
        return Err(Failure::unknown(e.to_string()));
    }
    Ok(tmp)
}
pub fn keys(expected: &str, values: &str) -> Result<Value, Failure> {
    if values.len() > LIMIT || values.contains('\0') {
        return Err(Failure::invalid("invalid authorized keys"));
    }
    if read(Path::new(KEYS))? != expected {
        return Err(Failure::invalid("keys changed; reload before saving"));
    }
    let tmp = write_new(Path::new(KEYS), values.as_bytes(), 0o600, None)?;
    fs::rename(&tmp, KEYS).map_err(|e| {
        let _ = fs::remove_file(&tmp);
        Failure::unknown(e.to_string())
    })?;
    Ok(json!({"result":true}))
}
pub fn certificate(cert: &str, key: &str) -> Result<Value, Failure> {
    if cert.len() > LIMIT
        || key.len() > LIMIT
        || !cert.starts_with("-----BEGIN CERTIFICATE-----")
        || !cert.contains("-----END CERTIFICATE-----")
        || !key.starts_with("-----BEGIN PRIVATE KEY-----")
        || !key.contains("-----END PRIVATE KEY-----")
    {
        return Err(Failure::invalid("invalid PEM certificate or key"));
    }
    // verso:root, as the verso-rpcd init script sets it on every start.
    install(Path::new(TLS_DIR), cert, key, Some((crate::VERSO_UID, 0)))?;
    announce()
}

// install replaces the pair in dir: the key first, readable by its owner
// alone, then the certificate. A certificate that cannot take its place puts
// the old key back, so the pair on disk always matches.
fn install(dir: &Path, cert: &str, key: &str, owner: Option<(u32, u32)>) -> Result<(), Failure> {
    let (certpath, keypath) = tls_files(dir);
    let oldkey = read_bytes(&keypath)?;
    let certtmp = write_new(&certpath, cert.as_bytes(), 0o644, None)?;
    let keytmp = match write_new(&keypath, key.as_bytes(), 0o600, owner) {
        Ok(p) => p,
        Err(e) => {
            let _ = fs::remove_file(&certtmp);
            return Err(e);
        }
    };
    if let Err(e) = fs::rename(&keytmp, &keypath) {
        let _ = fs::remove_file(&certtmp);
        let _ = fs::remove_file(&keytmp);
        return Err(Failure::unknown(e.to_string()));
    }
    if let Err(e) = fs::rename(&certtmp, &certpath) {
        if let Ok(tmp) = write_new(&keypath, &oldkey, 0o600, owner) {
            let _ = fs::rename(tmp, &keypath);
        }
        let _ = fs::remove_file(&certtmp);
        return Err(Failure::unknown(e.to_string()));
    }
    Ok(())
}

/// listener_shaped reports whether addr reads as address:port and nothing
/// else uci or a shell could take for more.
fn listener_shaped(addr: &str) -> bool {
    addr.len() <= 64
        && addr.contains(':')
        && addr
            .bytes()
            .all(|b| b.is_ascii_hexdigit() || matches!(b, b'.' | b':' | b'[' | b']'))
}

// uci runs one uci command, reporting whether it succeeded; what it prints is
// not the helper's to log.
fn uci(args: &[&str]) -> Result<bool, Failure> {
    Command::new("/sbin/uci")
        .args(args)
        .stdout(std::process::Stdio::null())
        .status()
        .map(|s| s.success())
        .map_err(|e| Failure::unknown(e.to_string()))
}

/// web_listeners writes verso.web's listener options — the HTTPS and HTTP
/// listeners, space-separated, and the redirect — and commits verso on its
/// own, so what a session stages elsewhere is left as it is. An empty value
/// clears its option. procd then records the shell's new command line; the
/// running shell already answers on the listeners (ADR-017 §1).
pub fn web_listeners(https: &str, http: &str, redirect: &str) -> Result<Value, Failure> {
    if !https
        .split_whitespace()
        .chain(http.split_whitespace())
        .all(listener_shaped)
        || !matches!(redirect, "" | "0" | "1")
    {
        return Err(Failure::invalid("invalid listener"));
    }
    if !uci(&["-q", "get", "verso.web"])? && !uci(&["set", "verso.web=web"])? {
        return Err(Failure::unknown("verso.web could not be created"));
    }
    for (option, addrs) in [("listen_https", https), ("listen_http", http)] {
        let path = format!("verso.web.{option}");
        uci(&["-q", "delete", &path])?;
        for addr in addrs.split_whitespace() {
            if !uci(&["add_list", &format!("{path}={addr}")])? {
                return Err(Failure::unknown(format!("{path} could not be written")));
            }
        }
    }
    if redirect.is_empty() {
        uci(&["-q", "delete", "verso.web.redirect_https"])?;
    } else if !uci(&["set", &format!("verso.web.redirect_https={redirect}")])? {
        return Err(Failure::unknown("verso.web.redirect_https could not be written"));
    }
    // Verso's own act, not an edit made outside it (ADR-021).
    crate::config_watch::own_write("verso");
    if !uci(&["commit", "verso"])? {
        return Err(Failure::unknown("verso could not be committed"));
    }
    let _ = Command::new("/etc/init.d/verso").arg("reload").status();
    Ok(json!({"result":true}))
}

/// set_theme puts a theme in force (ADR-024): it writes verso.ui.theme and
/// commits verso on its own, through root's save directory, so what an
/// operator has staged, which rpcd keeps per session, stays staged. Sand
/// following the browser is the option's absence. The service is not reloaded:
/// the running shell swaps its own theme, and the init script carries the
/// option to the next start.
pub fn set_theme(id: &str) -> Result<Value, Failure> {
    set_theme_with(id, uci)?;
    Ok(json!({"result": true}))
}

/// set_theme_with is set_theme over run, the way one uci command is run.
fn set_theme_with(id: &str, mut run: impl FnMut(&[&str]) -> Result<bool, Failure>) -> Result<(), Failure> {
    if id.is_empty() || id.len() > 64 || !id.bytes().all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-') {
        return Err(Failure::invalid("invalid theme"));
    }
    if !run(&["set", "verso.ui=ui"])? {
        return Err(Failure::unknown("verso.ui could not be created"));
    }
    if id == "sand" {
        run(&["-q", "delete", "verso.ui.theme"])?;
    } else if !run(&["set", &format!("verso.ui.theme={id}")])? {
        return Err(Failure::unknown("verso.ui.theme could not be written"));
    }
    // Verso's own act, not an edit made outside it (ADR-021).
    crate::config_watch::own_write("verso");
    if !run(&["commit", "verso"])? {
        return Err(Failure::unknown("verso could not be committed"));
    }
    Ok(())
}

/// make has the router make a new self-signed certificate, named as at first
/// boot for every way the LAN reaches it, by the init script's own step; the
/// key is born here, as root, and never crosses into the shell's request.
pub fn make() -> Result<Value, Failure> {
    let status = Command::new("/etc/init.d/verso")
        .arg("certificate")
        .status()
        .map_err(|e| Failure::unknown(e.to_string()))?;
    if !status.success() {
        return Err(Failure::unknown("the router could not make a certificate"));
    }
    announce()
}

// announce tells the running shell a new pair is on disk. procd delivers the
// signal; the shell reads the pair again and serves it from the next
// handshake, keeping every connection (ADR-017 §4).
fn announce() -> Result<Value, Failure> {
    let status = Command::new("/bin/ubus")
        .args(["call", "service", "signal", r#"{"name":"verso","signal":1}"#])
        .status()
        .map_err(|e| Failure::unknown(e.to_string()))?;
    if !status.success() {
        return Err(Failure::unknown(
            "certificate saved, but the web interface could not be told to serve it",
        ));
    }
    Ok(json!({"result":true}))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::unix::fs::PermissionsExt;
    #[test]
    fn certificate_and_key_use_distinct_private_temporary_files() {
        let dir = std::env::temp_dir().join(format!("verso-credentials-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let cert = write_new(&dir.join("tls.crt"), b"public", 0o644, None)
            .ok()
            .unwrap();
        let key = write_new(&dir.join("tls.key"), b"private", 0o600, None)
            .ok()
            .unwrap();
        assert_ne!(cert, key);
        assert_eq!(
            fs::metadata(&key).unwrap().permissions().mode() & 0o777,
            0o600
        );
        assert_eq!(
            fs::metadata(&cert).unwrap().permissions().mode() & 0o777,
            0o644
        );
        assert_eq!(fs::read(&cert).unwrap(), b"public");
        assert_eq!(fs::read(&key).unwrap(), b"private");
        fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn a_listener_is_an_address_and_a_port_and_nothing_more() {
        for good in ["0.0.0.0:443", "[::]:8443", "192.168.1.1:80", "[fd00::1]:9443", ":8443"] {
            assert!(listener_shaped(good), "{good}");
        }
        for bad in ["8443", "router:443", "0.0.0.0:443;reboot", "0.0.0.0:443 x", "a'b:1", ""] {
            assert!(!listener_shaped(bad), "{bad}");
        }
        assert!(web_listeners("0.0.0.0:443;x", "", "").is_err());
        assert!(web_listeners("", "", "yes").is_err());
    }

    /// The uci commands set_theme_with runs for id, in order.
    fn theme_commands(id: &str) -> Result<Vec<String>, Failure> {
        let mut ran = Vec::new();
        set_theme_with(id, |args| {
            ran.push(args.join(" "));
            Ok(true)
        })?;
        Ok(ran)
    }

    #[test]
    fn a_theme_is_written_and_committed_on_its_own() {
        assert_eq!(
            theme_commands("catppuccin-mocha").unwrap(),
            ["set verso.ui=ui", "set verso.ui.theme=catppuccin-mocha", "commit verso"]
        );
        // Sand following the browser is the option's absence.
        assert_eq!(theme_commands("sand").unwrap(), ["set verso.ui=ui", "-q delete verso.ui.theme", "commit verso"]);
    }

    #[test]
    fn a_theme_id_is_letters_digits_and_hyphens() {
        let long = "a".repeat(65);
        for bad in ["", "Mocha", "night;reboot", "../x", "a b", "night\n", long.as_str()] {
            assert!(theme_commands(bad).is_err(), "{bad:?} accepted");
        }
        assert!(theme_commands(&"a".repeat(64)).is_ok());
    }

    #[test]
    fn a_theme_that_does_not_commit_says_so() {
        let failed = set_theme_with("sand-dark", |args| Ok(args[0] != "commit"));
        assert!(failed.is_err());
    }

    #[test]
    fn an_installed_pair_replaces_the_one_on_disk_and_leaves_nothing_beside_it() {
        use std::os::unix::fs::MetadataExt;
        let dir = std::env::temp_dir().join(format!("verso-tls-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let (cert, key) = tls_files(&dir);
        fs::write(&cert, "old certificate").unwrap();
        fs::write(&key, "old key").unwrap();
        // The test's own account stands in for verso's, and another of its
        // groups, where it has one, for root's: the key is given to both.
        let uid = fs::metadata(&dir).unwrap().uid();
        let made_with = fs::metadata(&dir).unwrap().gid();
        let gid = fs::read_to_string("/proc/self/status")
            .unwrap()
            .lines()
            .find_map(|line| line.strip_prefix("Groups:"))
            .and_then(|groups| {
                groups
                    .split_whitespace()
                    .filter_map(|g| g.parse().ok())
                    .find(|&g| g != made_with)
            })
            .unwrap_or(made_with);
        install(&dir, "new certificate", "new key", Some((uid, gid)))
            .ok()
            .unwrap();
        assert_eq!(fs::read_to_string(&cert).unwrap(), "new certificate");
        assert_eq!(fs::read_to_string(&key).unwrap(), "new key");
        // The shell's account alone reads the key: plugins share its group.
        let key_meta = fs::metadata(&key).unwrap();
        assert_eq!(key_meta.permissions().mode() & 0o777, 0o600);
        assert_eq!((key_meta.uid(), key_meta.gid()), (uid, gid));
        assert_eq!(fs::read_dir(&dir).unwrap().count(), 2, "temporaries left");
        fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn a_key_that_cannot_be_given_to_its_owner_leaves_the_pair_as_it_was() {
        use std::os::unix::fs::MetadataExt;
        let dir = std::env::temp_dir().join(format!("verso-tls-owner-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let (uid, gid) = (fs::metadata(&dir).unwrap().uid(), fs::metadata(&dir).unwrap().gid());
        if uid == 0 {
            // Root may give a file to anyone.
            fs::remove_dir_all(dir).unwrap();
            return;
        }
        let (cert, key) = tls_files(&dir);
        fs::write(&cert, "old certificate").unwrap();
        fs::write(&key, "old key").unwrap();
        assert!(install(&dir, "new certificate", "new key", Some((uid + 1, gid))).is_err());
        assert_eq!(fs::read_to_string(&cert).unwrap(), "old certificate");
        assert_eq!(fs::read_to_string(&key).unwrap(), "old key");
        assert_eq!(fs::read_dir(&dir).unwrap().count(), 2, "temporaries left");
        fs::remove_dir_all(dir).unwrap();
    }
}
