// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The two text encodings a plugin reads and writes: standard base64 (the
//! shell's injected reads, an SSH key's blob) and a URL's percent-encoding.

const ALPHABET: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/// base64_decode reads standard base64, padded or not. Anything else — a
/// character outside the alphabet, padding mid-text, a length no encoder
/// writes — is no value at all.
pub fn base64_decode(text: &str) -> Option<Vec<u8>> {
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

/// url_encode percent-encodes every byte but RFC 3986's unreserved
/// characters, so a name rides a query string or a path segment unchanged.
pub fn url_encode(text: &str) -> String {
    text.bytes()
        .map(|b| {
            if b.is_ascii_alphanumeric() || b"-_.~".contains(&b) {
                (b as char).to_string()
            } else {
                format!("%{b:02X}")
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn base64_reads_padded_and_unpadded_text() {
        assert_eq!(base64_decode("TWFu").unwrap(), b"Man");
        assert_eq!(base64_decode("TWE=").unwrap(), b"Ma");
        assert_eq!(base64_decode("TQ==").unwrap(), b"M");
        assert_eq!(base64_decode("TQ").unwrap(), b"M");
        assert!(base64_decode("TQ=A").is_none());
        assert!(base64_decode("T!==").is_none());
        assert!(base64_decode("TQ===").is_none());
        assert!(base64_decode("TWF u").is_none());
    }

    #[test]
    fn a_url_carries_only_unreserved_characters_as_written() {
        assert_eq!(url_encode("br-lan_2.0~x"), "br-lan_2.0~x");
        assert_eq!(url_encode("a b/c&d=é"), "a%20b%2Fc%26d%3D%C3%A9");
    }
}
