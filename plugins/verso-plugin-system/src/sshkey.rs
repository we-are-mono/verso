// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! Reading an SSH public key the way `ssh-keygen -l` reads it: its size, its
//! SHA-256 fingerprint, what its owner called it and what kind it is. The
//! Access page shows the reading while a key is being pasted, so the person
//! adding it can hold it against the same line on their own machine. It is a
//! reading for the eye only: the shell parses the key again with its own SSH
//! library before anything is written.

use verso_plugin::sha256;

/// A pasted public key, read.
#[derive(Debug, PartialEq, Eq)]
pub struct KeyReading {
    pub bits: u32,
    pub kind: &'static str,
    pub comment: String,
    pub fingerprint: String,
}

impl KeyReading {
    /// summary is the reading as `ssh-keygen -l` prints it.
    pub fn summary(&self) -> String {
        let comment = match self.comment.as_str() {
            "" => "no comment",
            c => c,
        };
        format!(
            "{} {} {} ({})",
            self.bits, self.fingerprint, comment, self.kind
        )
    }
}

pub const UNREADABLE: &str = "Paste one complete SSH public key.";
pub const UNSUPPORTED: &str = "Use an ed25519, ECDSA or RSA key.";

/// read takes one pasted public key line and reads it, or says what is wrong
/// with it in words someone pasting a key can act on. The line's edges are
/// forgiven (a paste often brings a newline); its content is not: one key, its
/// label matching the kind its data says it is.
pub fn read(text: &str) -> Result<KeyReading, &'static str> {
    let line = text.trim();
    if line.is_empty() || line.lines().count() != 1 {
        return Err(UNREADABLE);
    }
    let (label, rest) = line.split_once(char::is_whitespace).ok_or(UNREADABLE)?;
    let rest = rest.trim_start();
    let (encoded, comment) = match rest.split_once(char::is_whitespace) {
        Some((encoded, comment)) => (encoded, comment.trim()),
        None => (rest, ""),
    };
    let blob = base64_decode(encoded).ok_or(UNREADABLE)?;
    let mut data = Wire(&blob);
    if data.string()? != label.as_bytes() {
        return Err(UNREADABLE);
    }
    let (kind, bits) = match label {
        "ssh-ed25519" => ("ED25519", 256),
        "sk-ssh-ed25519@openssh.com" => ("ED25519-SK", 256),
        "ssh-rsa" => {
            data.string()?; // the public exponent
            ("RSA", magnitude_bits(data.string()?))
        }
        "ecdsa-sha2-nistp256" | "ecdsa-sha2-nistp384" | "ecdsa-sha2-nistp521" => {
            let bits = match data.string()? {
                b"nistp256" => 256,
                b"nistp384" => 384,
                b"nistp521" => 521,
                _ => return Err(UNREADABLE),
            };
            ("ECDSA", bits)
        }
        "sk-ecdsa-sha2-nistp256@openssh.com" => ("ECDSA-SK", 256),
        _ => return Err(UNSUPPORTED),
    };
    Ok(KeyReading {
        bits,
        kind,
        comment: comment.into(),
        fingerprint: format!("SHA256:{}", base64_encode(&sha256(&blob))),
    })
}

/// Wire reads the length-prefixed strings an SSH key is written as.
struct Wire<'a>(&'a [u8]);

impl<'a> Wire<'a> {
    fn string(&mut self) -> Result<&'a [u8], &'static str> {
        let (len, rest) = self.0.split_at_checked(4).ok_or(UNREADABLE)?;
        let len = u32::from_be_bytes([len[0], len[1], len[2], len[3]]) as usize;
        let (value, rest) = rest.split_at_checked(len).ok_or(UNREADABLE)?;
        self.0 = rest;
        Ok(value)
    }
}

/// magnitude_bits is how many bits a big-endian unsigned integer holds, its
/// sign-padding zeros left out — an RSA key's size is its modulus's.
fn magnitude_bits(number: &[u8]) -> u32 {
    let significant = number.iter().skip_while(|b| **b == 0).collect::<Vec<_>>();
    match significant.first() {
        None => 0,
        Some(first) => (significant.len() as u32 - 1) * 8 + (8 - first.leading_zeros()),
    }
}

const ALPHABET: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/// base64_decode reads standard base64, padded or not.
fn base64_decode(text: &str) -> Option<Vec<u8>> {
    let body = text.trim_end_matches('=');
    if text.len() - body.len() > 2 || body.len() % 4 == 1 {
        return None;
    }
    let mut out = Vec::with_capacity(body.len() * 3 / 4);
    let (mut acc, mut held) = (0u32, 0u32);
    for c in body.bytes() {
        let value = ALPHABET.iter().position(|a| *a == c)? as u32;
        acc = (acc << 6) | value;
        held += 6;
        if held >= 8 {
            held -= 8;
            out.push((acc >> held) as u8);
            acc &= (1 << held) - 1;
        }
    }
    Some(out)
}

/// base64_encode writes standard base64 without padding, as fingerprints are.
fn base64_encode(data: &[u8]) -> String {
    let mut out = String::with_capacity(data.len().div_ceil(3) * 4);
    for chunk in data.chunks(3) {
        let n = chunk
            .iter()
            .enumerate()
            .fold(0u32, |n, (i, b)| n | (u32::from(*b) << (16 - 8 * i)));
        for i in 0..=chunk.len() {
            out.push(ALPHABET[(n >> (18 - 6 * i) & 63) as usize] as char);
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    const ED25519: &str = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG8/UqksrqlP7SLQWgj8xqnjV3e6cdgzDyzcNwOcT5+K demo@laptop";
    const RSA: &str = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCwGNPCD4xT9WS4G67DJuaBTKYR3ZgyliHwG5o0ObVR2q+VCBdgaRJc4RuiLZp5E6SrdFP+MjjAPLCDmHhq7AjjN5SR003gLz9E9Ndh+36KVyRoFxARuIlFSYqOaQGAFyClgTnJk67sYOCscw6wTaJLuqQ/7iD/BY3hTBItGVUPeeE4tfUtOlmx7IswX5GefmaPx/Lte5VKGJqOW8OoiSwT6mypwt7M0qD2bBMANxkD4zmZogmhMokEFGlxtUs+Q0Z1pt7OZYNPaISkGaAOzJjDrYYiRP8TnB4+JXRMhzneztKf8+Edzq3uTYPP9UXrw5PUNF73l2nvhQyhyrIb+KQr ci@buildbox";
    const ECDSA: &str = "ecdsa-sha2-nistp384 AAAAE2VjZHNhLXNoYTItbmlzdHAzODQAAAAIbmlzdHAzODQAAABhBI+tpyy873kmiPStjXTT/q26PVpqIMh9r9sB2VtVcxUrKLUg/XSfg/Bh6Td70ojpr9Bmudpq7gtPY2qu3r+ZCCNH0lBGEs0INQzSJH2LGN25uzFcd4NVXP9Zrdhzu4D3Rg==";

    #[test]
    fn base64_reads_padded_and_unpadded_text() {
        assert_eq!(base64_decode("TWFu").unwrap(), b"Man");
        assert_eq!(base64_decode("TWE=").unwrap(), b"Ma");
        assert_eq!(base64_decode("TQ==").unwrap(), b"M");
        assert!(base64_decode("TQ=A").is_none());
        assert!(base64_decode("T!==").is_none());
    }

    #[test]
    fn keys_read_as_ssh_keygen_reads_them() {
        for (line, want) in [
            (
                ED25519,
                "256 SHA256:UPedI7axeQlxL8dlMkeSROduLmrflVzNxJkm5UNiHxw demo@laptop (ED25519)",
            ),
            (
                RSA,
                "2048 SHA256:Zb3f14FlAAymIx1bz4WGNWncRCGUj/hEJGYy4YsZZag ci@buildbox (RSA)",
            ),
            (
                ECDSA,
                "384 SHA256:kGGFJibNyUFajFy+AEmC2bzGzCcCtVxVL8I85jAtFbU no comment (ECDSA)",
            ),
        ] {
            assert_eq!(
                read(line).map(|k| k.summary()),
                Ok(want.to_string()),
                "{line}"
            );
        }
    }

    #[test]
    fn a_paste_is_forgiven_its_edges_but_not_its_content() {
        let padded = format!("  {ED25519}\n");
        assert_eq!(read(&padded).unwrap().comment, "demo@laptop");
        let spaced = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG8/UqksrqlP7SLQWgj8xqnjV3e6cdgzDyzcNwOcT5+K my work laptop";
        assert_eq!(read(spaced).unwrap().comment, "my work laptop");
        for bad in [
            "",
            "   ",
            "ssh-ed25519",
            "ssh-ed25519 not-base64!",
            &format!("{ED25519}\n{RSA}"),
            "ssh-dss AAAAB3NzaC1kc3M=",
            // a blob that says it is RSA under an ed25519 label
            &RSA.replacen("ssh-rsa", "ssh-ed25519", 1),
        ] {
            assert!(read(bad).is_err(), "{bad:?}");
        }
    }
}
