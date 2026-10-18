// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! What a path is, as the helper must know it before it reads, writes or runs
//! what the path names. Every walk goes through one seam, [`Owner`], so the
//! rules are tested without root and without the files.

use std::io;
use std::os::unix::fs::{MetadataExt, PermissionsExt};
use std::path::Path;

/// The facts about one path a check needs, without following a link.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Meta {
    pub link: bool,
    pub file: bool,
    pub uid: u32,
    pub gid: u32,
    pub mode: u32,
}

/// Owner answers what a path is.
pub trait Owner {
    fn meta(&self, path: &Path) -> io::Result<Meta>;
}

/// The real filesystem.
pub struct System;

impl Owner for System {
    fn meta(&self, path: &Path) -> io::Result<Meta> {
        let m = std::fs::symlink_metadata(path)?;
        Ok(Meta {
            link: m.file_type().is_symlink(),
            file: m.file_type().is_file(),
            uid: m.uid(),
            gid: m.gid(),
            mode: m.permissions().mode(),
        })
    }
}

/// Why [`no_links`] refused a path.
#[derive(Debug, PartialEq, Eq)]
pub enum Refused {
    /// A part of the path is a symbolic link.
    Link,
    /// A part of the path could not be read.
    Unreadable(String),
}

/// no_links refuses a path any part of which is a symbolic link. A part that
/// does not exist yet is no link.
pub fn no_links(owner: &impl Owner, path: &Path) -> Result<(), Refused> {
    for p in path.ancestors() {
        match owner.meta(p) {
            Ok(m) if m.link => return Err(Refused::Link),
            Ok(_) => {}
            Err(e) if e.kind() == io::ErrorKind::NotFound => {}
            Err(e) => return Err(Refused::Unreadable(format!("{}: {e}", p.display()))),
        }
    }
    Ok(())
}

/// root_only refuses a path unless it and every directory above it exist, are
/// owned by root, are writable by root alone and are not links, and the path
/// itself is a regular file: ubusd's rule for its ACL files, applied to what
/// the helper reads a command from and runs as root. A part its group may
/// write is root's alone when that group is root's own (an image built with
/// umask 002 leaves /usr so).
pub fn root_only(owner: &impl Owner, path: &Path) -> Result<(), String> {
    if !path.is_absolute() {
        return Err(format!("{} is not an absolute path", path.display()));
    }
    for (i, p) in path.ancestors().enumerate() {
        let m = owner.meta(p).map_err(|e| format!("{}: {e}", p.display()))?;
        if m.link {
            return Err(format!("{} is a symbolic link", p.display()));
        }
        if m.uid != 0 {
            return Err(format!("{} is not owned by root", p.display()));
        }
        if m.mode & 0o002 != 0 || (m.mode & 0o020 != 0 && m.gid != 0) {
            return Err(format!("{} is writable by others than root", p.display()));
        }
        if i == 0 && !m.file {
            return Err(format!("{} is not a regular file", p.display()));
        }
    }
    Ok(())
}

#[cfg(test)]
pub mod fake {
    use super::*;
    use std::collections::BTreeMap;
    use std::path::PathBuf;

    /// A filesystem as a table: every path root's, 0755 for directories and
    /// 0644 for files, unless set otherwise.
    #[derive(Default)]
    pub struct Table(pub BTreeMap<PathBuf, Meta>);

    impl Table {
        pub fn dir(mut self, path: &str) -> Table {
            self.0.insert(path.into(), Meta { link: false, file: false, uid: 0, gid: 0, mode: 0o40755 });
            self
        }
        pub fn file(mut self, path: &str) -> Table {
            self.0.insert(path.into(), Meta { link: false, file: true, uid: 0, gid: 0, mode: 0o100644 });
            self
        }
        pub fn set(mut self, path: &str, f: impl FnOnce(&mut Meta)) -> Table {
            f(self.0.get_mut(Path::new(path)).expect("a path in the table"));
            self
        }
        /// A tree holding `files`, with each directory above them.
        pub fn tree(files: &[&str]) -> Table {
            let mut t = Table::default();
            for f in files {
                for a in Path::new(f).ancestors().skip(1) {
                    t = t.dir(a.to_str().unwrap());
                }
                t = t.file(f);
            }
            t
        }
    }

    impl Owner for Table {
        fn meta(&self, path: &Path) -> io::Result<Meta> {
            self.0
                .get(path)
                .copied()
                .ok_or_else(|| io::Error::from(io::ErrorKind::NotFound))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::fake::Table;
    use super::*;

    const EXEC: &str = "/usr/bin/verso-plugin-ddns";

    #[test]
    fn a_root_owned_file_passes() {
        assert_eq!(root_only(&Table::tree(&[EXEC]), Path::new(EXEC)), Ok(()));
    }

    #[test]
    fn a_file_or_parent_another_account_may_write_is_refused() {
        let group = |m: &mut Meta| {
            m.mode = 0o100775;
            m.gid = 6000;
        };
        let t = Table::tree(&[EXEC]).set(EXEC, group);
        assert!(root_only(&t, Path::new(EXEC)).unwrap_err().contains(EXEC));
        let t = Table::tree(&[EXEC]).set("/usr/bin", |m| m.mode = 0o40777);
        assert!(root_only(&t, Path::new(EXEC)).unwrap_err().contains("/usr/bin"));
    }

    #[test]
    fn root_group_write_is_root_alone() {
        let t = Table::tree(&[EXEC]).set("/usr", |m| m.mode = 0o40775);
        assert_eq!(root_only(&t, Path::new(EXEC)), Ok(()));
    }

    #[test]
    fn another_owner_is_refused() {
        let t = Table::tree(&[EXEC]).set("/usr", |m| m.uid = 6013);
        assert!(root_only(&t, Path::new(EXEC)).unwrap_err().contains("not owned by root"));
    }

    #[test]
    fn a_link_is_refused() {
        let t = Table::tree(&[EXEC]).set(EXEC, |m| m.link = true);
        assert!(root_only(&t, Path::new(EXEC)).unwrap_err().contains("symbolic link"));
    }

    #[test]
    fn a_directory_or_a_missing_file_is_refused() {
        assert!(root_only(&Table::tree(&[EXEC]), Path::new("/usr/bin")).is_err());
        assert!(root_only(&Table::tree(&[EXEC]), Path::new("/usr/bin/other")).is_err());
        assert!(root_only(&Table::tree(&[EXEC]), Path::new("usr/bin/x")).is_err());
    }

    #[test]
    fn no_links_passes_a_path_not_made_yet() {
        let t = Table::tree(&["/etc/nftables.d/a.nft"]);
        assert_eq!(no_links(&t, Path::new("/etc/nftables.d/new.nft")), Ok(()));
        let t = t.set("/etc/nftables.d", |m| m.link = true);
        assert_eq!(no_links(&t, Path::new("/etc/nftables.d/new.nft")), Err(Refused::Link));
    }
}
