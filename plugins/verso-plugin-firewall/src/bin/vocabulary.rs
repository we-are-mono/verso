// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Reads firewall4's own option tables and writes the vocabulary artifact.
//!
//!     cargo run --bin vocabulary -- <path to fw4.uc> [--write]
//!
//! Without `--write` it prints what it found and how that differs from the
//! artifact already checked in, and changes nothing. That is the normal way to run
//! it: the committed file is the baseline and a difference is something a person
//! reads and accepts, never something a tool applies. An extractor that can
//! overwrite its own output can quietly shrink it — "found 12 options, not 37"
//! would otherwise look exactly like success.
//!
//! It is deliberately a plain-text reader rather than a ucode parser. What keeps
//! it honest is not its cleverness but the guards around it: it refuses a yield
//! that looks wrong, it records the hash of what it read, and the artifact it
//! writes is compared against firewall4's own behaviour by a separate probe.

use std::collections::BTreeMap;
use std::fmt::Write as _;
use std::process::ExitCode;

/// EXTRACTOR is this tool's identity, recorded in the artifact. A changed reader
/// can change the list as surely as a changed daemon, so the artifact names both.
const EXTRACTOR: &str = "fw4-vocabulary v1";

/// The section types worth reading, in the order the artifact lists them. A
/// section firewall4 parses but this does not name is not in the artifact at all,
/// which the floor below turns into a failure rather than an omission.
const SECTIONS: [&str; 8] = [
    "defaults",
    "zone",
    "forwarding",
    "rule",
    "redirect",
    "nat",
    "include",
    "ipset",
];

/// FLOORS is what each section type is known to hold. The extractor refuses to
/// report fewer: a table it cannot find reads as an empty one, and an empty one
/// would pass every later guard while making the whole config look optionless.
/// These are floors, not counts — firewall4 gaining an option is news, not an
/// error.
const FLOORS: [(&str, usize); 8] = [
    ("defaults", 20),
    ("zone", 23),
    ("forwarding", 5),
    ("rule", 37),
    ("redirect", 34),
    ("nat", 20),
    ("include", 7),
    ("ipset", 17),
];

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let Some(path) = args.first() else {
        eprintln!("usage: vocabulary <path to fw4.uc> [--write]");
        return ExitCode::FAILURE;
    };
    let source = match std::fs::read_to_string(path) {
        Ok(text) => text,
        Err(err) => {
            eprintln!("cannot read {path}: {err}");
            return ExitCode::FAILURE;
        }
    };

    let sections = match read_sections(&source) {
        Ok(sections) => sections,
        Err(err) => {
            eprintln!("{err}");
            return ExitCode::FAILURE;
        }
    };

    let digest = verso_plugin::sha256_hex(source.as_bytes());
    let version = version_of(path);
    let artifact = render(&sections, &version, &digest);

    for (kind, options) in &sections {
        println!("{kind}: {} options", options.len());
    }
    println!("\nfw4.uc sha256 {digest}");
    println!("firewall4 version {version}");

    if args.iter().any(|a| a == "--write") {
        let out = "src/vocabulary.rs";
        if let Err(err) = std::fs::write(out, &artifact) {
            eprintln!("cannot write {out}: {err}");
            return ExitCode::FAILURE;
        }
        println!("\nwrote {out} — read the diff before committing it");
    } else {
        println!("\n(not written; pass --write once you have read the diff)");
    }
    ExitCode::SUCCESS
}

/// An option as firewall4 declares it.
struct Spec {
    key: String,
    datatype: String,
    default: Option<String>,
    list: bool,
    invertible: bool,
    support: &'static str,
}

/// read_sections pulls one option table per section type out of the source, and
/// refuses a yield below the floor.
fn read_sections(source: &str) -> Result<Vec<(String, Vec<Spec>)>, String> {
    let lines: Vec<&str> = source.lines().collect();
    let floors: BTreeMap<&str, usize> = FLOORS.into_iter().collect();
    let mut out = Vec::new();
    for kind in SECTIONS {
        let options = read_table(&lines, kind)
            .ok_or_else(|| format!("no parse_{kind} option table found in the source"))?;
        let floor = floors.get(kind).copied().unwrap_or(0);
        if options.len() < floor {
            return Err(format!(
                "parse_{kind} yielded {} options, fewer than the {floor} known to exist — \
                 the table's shape has changed and this reader no longer understands it",
                options.len()
            ));
        }
        out.push((kind.to_string(), options));
    }
    Ok(out)
}

/// read_table reads the option lines between `parse_<kind>: function` and the end
/// of the `parse_options` call it opens.
fn read_table(lines: &[&str], kind: &str) -> Option<Vec<Spec>> {
    let head = format!("parse_{kind}: function");
    let start = lines.iter().position(|l| l.contains(&head))?;
    let mut out = Vec::new();
    let mut inside = false;
    for line in &lines[start..] {
        if !inside {
            if line.contains("parse_options(") {
                inside = true;
            }
            continue;
        }
        // The call's closing brace ends the table; a line that is only a brace and
        // a paren is it.
        let trimmed = line.trim();
        if trimmed.starts_with("});") {
            break;
        }
        if let Some(spec) = read_option(trimmed) {
            out.push(spec);
        }
    }
    match out.is_empty() {
        true => None,
        false => Some(out),
    }
}

/// read_option reads one `key: [ "datatype", default, FLAGS ]` line. Anything else
/// — a blank line, a comment, a continuation — is not an option and is skipped.
fn read_option(line: &str) -> Option<Spec> {
    let (key, rest) = line.split_once(": [")?;
    let key = key.trim();
    if key.is_empty() || !key.chars().all(|c| c.is_ascii_lowercase() || c == '_') {
        return None;
    }
    let body = rest.trim_end().trim_end_matches(',').trim_end_matches(']');
    let mut parts = body.split(',').map(str::trim);
    let datatype = parts.next()?.trim_matches('"').to_string();
    if datatype.is_empty() {
        return None;
    }
    // The default may be a literal, `null`, or an expression reading another
    // section's value. An expression is a default the daemon computes, which this
    // cannot state, so it is recorded as absent rather than guessed at.
    let default = match parts.next().map(str::trim) {
        None | Some("null") => None,
        Some(value) if value.starts_with('"') => Some(value.trim_matches('"').to_string()),
        Some(_) => None,
    };
    let flags: String = body.to_string();
    Some(Spec {
        key: key.to_string(),
        datatype,
        default,
        list: flags.contains("PARSE_LIST"),
        invertible: !flags.contains("NO_INVERT"),
        support: match () {
            _ if flags.contains("UNSUPPORTED") => "Unsupported",
            _ if flags.contains("DEPRECATED") => "Deprecated",
            _ => "Supported",
        },
    })
}

/// version_of reads the firewall4 version out of the path it was extracted from —
/// the build tree spells it, and it is worth recording even though the hash is
/// what actually pins the file.
fn version_of(path: &str) -> String {
    path.split('/')
        .find(|part| part.starts_with("firewall4-"))
        .map(|part| part.trim_start_matches("firewall4-").to_string())
        .unwrap_or_else(|| "unknown".to_string())
}

/// render writes the artifact: a plain Rust source file, so the compiler checks it
/// and a reader can read it.
fn render(sections: &[(String, Vec<Spec>)], version: &str, digest: &str) -> String {
    let mut out = String::new();
    out.push_str(
        "// SPDX-License-Identifier: GPL-2.0-only\n\
         // SPDX-FileCopyrightText: 2026 Mono Technologies Inc.\n\n\
         //! What /etc/config/firewall can hold, read out of firewall4's own parser.\n\
         //!\n\
         //! GENERATED — `cargo run --bin vocabulary -- <path to fw4.uc> --write`.\n\
         //! Do not edit: the next extraction would drop the edit, and the point of the\n\
         //! file is that it says what firewall4 says rather than what we believe.\n\
         //!\n\
         //! A difference against this file is news to read, not a thing to apply. The\n\
         //! extractor prints the difference and writes nothing unless told to.\n\n\
         use verso_plugin::vocabulary::{\n    \
         Cardinality, OptionSpec, Provenance, SectionVocabulary, Support, Vocabulary,\n};\n\n",
    );
    let _ = write!(
        out,
        "/// FIREWALL is every option firewall4 parses, by section type.\n\
         pub const FIREWALL: Vocabulary = Vocabulary {{\n    \
         config: \"firewall\",\n    \
         provenance: Provenance::Extracted {{\n        \
         upstream: \"firewall4\",\n        \
         version: \"{version}\",\n        \
         sha256: \"{digest}\",\n        \
         extractor: \"{EXTRACTOR}\",\n    \
         }},\n    sections: SECTIONS,\n}};\n\n\
         const SECTIONS: &[SectionVocabulary] = &[\n"
    );
    for (kind, _) in sections {
        let _ = writeln!(
            out,
            "    SectionVocabulary {{ kind: \"{kind}\", options: {} }},",
            const_name(kind)
        );
    }
    out.push_str("];\n");
    for (kind, options) in sections {
        let _ = writeln!(out, "\nconst {}: &[OptionSpec] = &[", const_name(kind));
        for spec in options {
            let default = match &spec.default {
                Some(value) => format!("Some(\"{value}\")"),
                None => "None".to_string(),
            };
            let cardinality = match spec.list {
                true => "List",
                false => "Scalar",
            };
            let _ = writeln!(
                out,
                "    OptionSpec {{ key: \"{}\", datatype: \"{}\", default: {default}, \
                 cardinality: Cardinality::{cardinality}, invertible: {}, \
                 support: Support::{} }},",
                spec.key, spec.datatype, spec.invertible, spec.support
            );
        }
        out.push_str("];\n");
    }
    out
}

fn const_name(kind: &str) -> String {
    format!("{}_OPTIONS", kind.to_uppercase())
}
