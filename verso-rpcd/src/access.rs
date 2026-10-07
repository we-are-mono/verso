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
/// The certificate Verso serves HTTPS with (ADR-017 §3): the pair's directory,
/// and the group the shell runs in, which alone may read the key.
const TLS_DIR: &str = "/etc/verso";
const VERSO_GID: u32 = 6000;
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
// given, group set before a byte is written: the file is never readable by
// anyone it is not meant for, even before it is renamed into place.
fn write_new(path: &Path, value: &[u8], mode: u32, group: Option<u32>) -> Result<PathBuf, Failure> {
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
    let written = group
        .map_or(Ok(()), |gid| std::os::unix::fs::fchown(&f, None, Some(gid)))
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
    install(Path::new(TLS_DIR), cert, key, Some(VERSO_GID))?;
    announce()
}

// install replaces the pair in dir: the key first, readable by group alone,
// then the certificate. A certificate that cannot take its place puts the old
// key back, so the pair on disk always matches.
fn install(dir: &Path, cert: &str, key: &str, group: Option<u32>) -> Result<(), Failure> {
    let (certpath, keypath) = tls_files(dir);
    let oldkey = read_bytes(&keypath)?;
    let certtmp = write_new(&certpath, cert.as_bytes(), 0o644, None)?;
    let keytmp = match write_new(&keypath, key.as_bytes(), 0o640, group) {
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
        if let Ok(tmp) = write_new(&keypath, &oldkey, 0o640, group) {
            let _ = fs::rename(tmp, &keypath);
        }
        let _ = fs::remove_file(&certtmp);
        return Err(Failure::unknown(e.to_string()));
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
        let key = write_new(&dir.join("tls.key"), b"private", 0o640, None)
            .ok()
            .unwrap();
        assert_ne!(cert, key);
        assert_eq!(
            fs::metadata(&key).unwrap().permissions().mode() & 0o777,
            0o640
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
    fn an_installed_pair_replaces_the_one_on_disk_and_leaves_nothing_beside_it() {
        use std::os::unix::fs::MetadataExt;
        let dir = std::env::temp_dir().join(format!("verso-tls-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let (cert, key) = tls_files(&dir);
        fs::write(&cert, "old certificate").unwrap();
        fs::write(&key, "old key").unwrap();
        // The test's own group stands in for verso's: the key is given to it.
        let gid = fs::metadata(&dir).unwrap().gid();
        install(&dir, "new certificate", "new key", Some(gid))
            .ok()
            .unwrap();
        assert_eq!(fs::read_to_string(&cert).unwrap(), "new certificate");
        assert_eq!(fs::read_to_string(&key).unwrap(), "new key");
        let key_meta = fs::metadata(&key).unwrap();
        assert_eq!(key_meta.permissions().mode() & 0o777, 0o640);
        assert_eq!(key_meta.gid(), gid);
        assert_eq!(fs::read_dir(&dir).unwrap().count(), 2, "temporaries left");
        fs::remove_dir_all(dir).unwrap();
    }
}
