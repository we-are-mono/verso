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
fn configured(option: &str) -> Result<PathBuf, Failure> {
    let output = Command::new("/sbin/uci")
        .args(["-q", "get", &format!("uhttpd.@uhttpd[0].{option}")])
        .output()
        .map_err(|e| Failure::unknown(e.to_string()))?;
    if !output.status.success() {
        return Err(Failure::invalid("no configured web certificate"));
    }
    let path = String::from_utf8_lossy(&output.stdout).trim().to_string();
    if !path.starts_with("/etc/") || path.contains("..") || path.contains(['\n', '\r']) {
        return Err(Failure::invalid("certificate path must be under /etc"));
    }
    Ok(path.into())
}
pub fn state() -> Result<Value, Failure> {
    let keys = read(Path::new(KEYS))?;
    let cert = configured("cert").ok();
    let pem = cert
        .as_ref()
        .map(|p| read_bytes(p))
        .transpose()?
        .unwrap_or_default();
    Ok(
        json!({"authorized_keys":keys,"certificate_bytes":pem,"certificate_file":cert.map(|p|p.to_string_lossy().to_string()).unwrap_or_default()}),
    )
}
fn write_new(path: &Path, value: &[u8], mode: u32) -> Result<PathBuf, Failure> {
    let mut filename = path
        .file_name()
        .ok_or_else(|| Failure::invalid("invalid destination"))?
        .to_os_string();
    filename.push(format!(".verso-{}", std::process::id()));
    let tmp = path.with_file_name(filename);
    let mut f = OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(mode)
        .open(&tmp)
        .map_err(|e| Failure::unknown(e.to_string()))?;
    if let Err(e) = f.write_all(value).and_then(|_| f.sync_all()) {
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
    let tmp = write_new(Path::new(KEYS), values.as_bytes(), 0o600)?;
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
    let certpath = configured("cert")?;
    let keypath = configured("key")?;
    if certpath == keypath {
        return Err(Failure::invalid("certificate and key paths must differ"));
    }
    let oldkey = read_bytes(&keypath)?;
    let certtmp = write_new(&certpath, cert.as_bytes(), 0o644)?;
    let keytmp = match write_new(&keypath, key.as_bytes(), 0o600) {
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
        if let Ok(tmp) = write_new(&keypath, &oldkey, 0o600) {
            let _ = fs::rename(tmp, &keypath);
        }
        let _ = fs::remove_file(&certtmp);
        return Err(Failure::unknown(e.to_string()));
    }
    let status = Command::new("/etc/init.d/uhttpd")
        .arg("restart")
        .status()
        .map_err(|e| Failure::unknown(e.to_string()))?;
    if !status.success() {
        return Err(Failure::unknown(
            "certificate saved, but uhttpd could not reload",
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
        let cert = write_new(&dir.join("uhttpd.crt"), b"public", 0o644)
            .ok()
            .unwrap();
        let key = write_new(&dir.join("uhttpd.key"), b"private", 0o600)
            .ok()
            .unwrap();
        assert_ne!(cert, key);
        assert_eq!(
            fs::metadata(&key).unwrap().permissions().mode() & 0o777,
            0o600
        );
        assert_eq!(fs::read(&cert).unwrap(), b"public");
        assert_eq!(fs::read(&key).unwrap(), b"private");
        fs::remove_dir_all(dir).unwrap();
    }
}
