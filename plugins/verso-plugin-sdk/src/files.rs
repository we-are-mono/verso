// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! A set of hand-edited files a page lists and edits in place — dnsmasq's
//! option files, fw4's rule files. Every such set reads and edits the same
//! way, whichever daemon reads it, because it is drawn here once: a Files band
//! with its count and a New file act, a row per file with its folder before its
//! name, and an editor in a drawer whose save is staged through the shell's
//! file journal (the `config-file-stage` command) and applied, checked and
//! rolled back with everything else on the stage.
use crate::{
    ApplyAction, Form, RowDrawer, TableCell, TableColumn, TableGroup, TableRow, TableRowAct, Value,
    Widget,
};

/// NEW_FILE_VERSION is the version the helper gives a file that does not exist
/// yet: what an editor for a new file expects to replace.
pub const NEW_FILE_VERSION: &str = "af63bd4c8601b7df";

/// EDIT_ICON is the glyph a file's row opens its editor with — the pen every
/// edit act in Verso wears, not a chevron, which reads as going somewhere.
const EDIT_ICON: &str = "square-pen";

/// LIMIT is the most text one file may hold.
pub const LIMIT: usize = 32768;

/// FileSet is one daemon's hand-edited files as a page offers them: where
/// they live, what they are called, and where their editors are.
pub struct FileSet {
    /// The page the listing sits on; a closed editor returns to it.
    pub page: &'static str,
    /// The address of a file's editor, before the file's name.
    pub editors: &'static str,
    /// The folder the set's files live in, and the suffix each one has.
    pub dir: &'static str,
    pub suffix: &'static str,
    /// A file of the set outside its folder, which its editor addresses as
    /// "main" — dnsmasq's own configuration file.
    pub main: Option<&'static str>,
    /// What one line of a file is, said of one and of several.
    pub line: (&'static str, &'static str),
    /// What the listing says with no files in it.
    pub empty: &'static str,
    /// The refusal for a file larger than LIMIT.
    pub too_large: &'static str,
    /// The name a new file's box suggests.
    pub placeholder: &'static str,
}

/// files is the set's files as the helper reported them, out of a ubus read.
pub fn files(state: Option<&Value>) -> Vec<Value> {
    state
        .and_then(|s| s.get("files"))
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default()
}

fn text<'a>(f: &'a Value, key: &str) -> &'a str {
    f.get(key).and_then(Value::as_str).unwrap_or("")
}

impl FileSet {
    /// href is the address of a file's editor.
    pub fn href(&self, path: &str) -> String {
        let name = match self.main {
            Some(main) if main == path => "main",
            _ => path.strip_prefix(self.dir).unwrap_or(""),
        };
        format!("{}{name}", self.editors)
    }

    /// path is the file an editor's address names — "" for a new one — or
    /// None for an address that names no file of the set.
    pub fn path(&self, name: &str) -> Option<String> {
        match (name, self.main) {
            ("main", Some(main)) => Some(main.into()),
            ("new", _) => Some(String::new()),
            _ if name.strip_suffix(self.suffix).is_some_and(valid_name) => {
                Some(format!("{}{name}", self.dir))
            }
            _ => None,
        }
    }

    /// listing is the set's files: the Files band with its count and its New
    /// file act, then a row per file — its folder before its name, how many
    /// lines of setting it holds, and the way into its editor.
    pub fn listing(&self, files: &[Value]) -> Widget {
        let mut rows: Vec<TableRow> = files
            .iter()
            .map(|f| {
                let path = text(f, "path");
                let count = text(f, "content")
                    .lines()
                    .filter(|l| !l.trim().is_empty() && !l.trim().starts_with('#'))
                    .count();
                TableRow {
                    id: path.into(),
                    panel: self.href(path),
                    cells: vec![
                        TableCell {
                            text: path.rsplit('/').next().unwrap_or(path).into(),
                            sub: path
                                .rsplit_once('/')
                                .map(|(d, _)| format!("{d}/"))
                                .unwrap_or_default(),
                            ..Default::default()
                        },
                        TableCell {
                            text: count.to_string(),
                            sub: if count == 1 { self.line.0 } else { self.line.1 }.into(),
                            ..Default::default()
                        },
                        TableCell {
                            actions: vec![TableRowAct {
                                title: "Edit".into(),
                                icon: EDIT_ICON.into(),
                                href: self.href(path),
                                ..Default::default()
                            }],
                            ..Default::default()
                        },
                    ],
                    ..Default::default()
                }
            })
            .collect();
        // With no files the band still stands, alone: the table says its
        // empty sentence under it.
        if rows.is_empty() {
            rows.push(TableRow::default());
        }
        rows[0].group = Some(TableGroup {
            key: String::new(),
            label: "Files".into(),
            to: String::new(),
            chain: files.len().to_string(),
            tally: String::new(),
            add_label: "New file".into(),
            add_text: "New file".into(),
            add_href: format!("{}new", self.editors),
            add_panel: true,
        });
        table(
            ["path", "count", "actions"]
                .iter()
                .map(|kind| TableColumn {
                    kind: (*kind).into(),
                    ..TableColumn::default()
                })
                .collect(),
            rows,
            self.empty,
            "Edit",
        )
    }

    /// editor is one file's editor, open in its drawer: a new file asks for
    /// its name, with the suffix every file of the set has riding the box.
    pub fn editor(
        &self,
        path: &str,
        name: &str,
        content: &str,
        expected: &str,
        error: &str,
    ) -> Widget {
        let mut fields = vec![Widget::hidden("expected", expected)];
        if path.is_empty() {
            let mut filename = Widget::field("filename", "File name", name, "", "");
            if let Widget::Field {
                unit, placeholder, ..
            } = &mut filename
            {
                *unit = self.suffix.into();
                *placeholder = self.placeholder.into();
            }
            fields.push(filename);
        }
        let mut body = Widget::field("content", "Contents", content, "", "");
        if let Widget::Field { kind, style, .. } = &mut body {
            *kind = "textarea".into();
            *style = "code".into();
        }
        fields.push(body);
        fields.push(Widget::Text {
            markdown: format!("`{}`", if path.is_empty() { self.dir } else { path }),
        });
        let form = Widget::Form {
            style: "settings".into(),
            submit: "Save".into(),
            error: error.into(),
            fields,
            note: String::new(),
            target: String::new(),
        };
        let drawer = RowDrawer {
            title: if path.is_empty() {
                "New file"
            } else {
                "Edit file"
            }
            .into(),
            open: true,
            closed: self.page.into(),
            children: vec![form],
            ..Default::default()
        };
        table(
            vec![],
            vec![TableRow {
                drawer: Some(drawer),
                ..Default::default()
            }],
            "",
            "",
        )
    }

    /// open is the editor an address names, or why there is none: the file
    /// is unreadable, or gone.
    pub fn open(&self, files: &[Value], name: &str) -> Result<Widget, String> {
        let Some(path) = self.path(name) else {
            return Err("The file is no longer available.".into());
        };
        if path.is_empty() {
            return Ok(self.editor("", "", "", NEW_FILE_VERSION, ""));
        }
        match files.iter().find(|f| text(f, "path") == path) {
            Some(file) if !text(file, "error").is_empty() => Err(text(file, "error").into()),
            Some(file) => {
                Ok(self.editor(&path, "", text(file, "content"), text(file, "version"), ""))
            }
            None => Err("The file is no longer available.".into()),
        }
    }

    /// save answers an editor's submission: the editor again, with a refusal
    /// when the name or the text cannot be a file of the set, and otherwise
    /// the command that stages the file.
    pub fn save(
        &self,
        files: &[Value],
        name: &str,
        form: &Form,
    ) -> Option<(Widget, Option<ApplyAction>)> {
        let original = self.path(name)?;
        let filename = form.get("filename").trim().to_owned();
        let content = form.get("content");
        let expected = form.get("expected");
        let target = if original.is_empty() {
            format!("{}{filename}{}", self.dir, self.suffix)
        } else {
            original.clone()
        };
        let error = if original.is_empty() && !valid_name(&filename) {
            "Use lowercase letters, numbers, hyphens or underscores for the file name."
        } else if content.len() > LIMIT || content.contains('\0') {
            self.too_large
        } else if original.is_empty() && files.iter().any(|file| text(file, "path") == target) {
            "A file with this name already exists."
        } else {
            ""
        };
        let editor = self.editor(&original, &filename, &content, &expected, error);
        let command = error.is_empty().then(|| ApplyAction {
            name: "config-file-stage".into(),
            args: [
                ("path".into(), target),
                ("expected".into(), expected),
                ("content".into(), content),
            ]
            .into(),
        });
        Some((editor, command))
    }
}

/// valid_name is a file name as a person would write one: lowercase letters,
/// digits, dashes and underscores, starting with a letter or digit.
pub fn valid_name(s: &str) -> bool {
    !s.is_empty()
        && s.len() <= 64
        && s.as_bytes()[0].is_ascii_alphanumeric()
        && s.bytes()
            .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-' || b == b'_')
}

fn table(columns: Vec<TableColumn>, rows: Vec<TableRow>, empty: &str, drawer: &str) -> Widget {
    Widget::Table {
        style: String::new(),
        title: String::new(),
        detail: String::new(),
        dense: false,
        reorder_config: String::new(),
        reorder_label: String::new(),
        columns,
        rows,
        drawer_label: drawer.into(),
        drawer_icon: match drawer.is_empty() {
            true => String::new(),
            false => EDIT_ICON.into(),
        },
        empty_text: empty.into(),
        add_label: String::new(),
        add_href: String::new(),
        note: String::new(),
        stream: None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::json;

    const SET: FileSet = FileSet {
        page: "/plugins/x/",
        editors: "/plugins/x/files/",
        dir: "/etc/x.d/",
        suffix: ".conf",
        main: Some("/etc/x.conf"),
        line: ("option", "options"),
        empty: "No files",
        too_large: "Too large.",
        placeholder: "10-local",
    };

    // With no files, the band still stands — its name, its count, its add —
    // and the table says it has nothing under it in its own words.
    #[test]
    fn with_no_files_the_band_stands_alone_and_the_table_says_so() {
        let body = serde_json::to_value(SET.listing(&[])).unwrap();
        let rows = body["rows"].as_array().unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0]["group"]["label"], "Files");
        assert_eq!(rows[0]["group"]["chain"], "0");
        assert_eq!(rows[0]["group"]["add_href"], "/plugins/x/files/new");
        assert!(rows[0]["cells"].as_array().is_none_or(Vec::is_empty));
        assert_eq!(body["empty_text"], "No files");
    }

    // A row is a file: its folder before its name, the lines that set
    // something (comments and blanks are not), and its editor's address.
    #[test]
    fn a_row_is_a_file_with_its_folder_its_count_and_its_editor() {
        let files =
            [json!({"path":"/etc/x.d/10-a.conf","content":"# note\na\n\nb\n","version":"v"})];
        let body = serde_json::to_value(SET.listing(&files)).unwrap();
        let row = &body["rows"][0];
        assert_eq!(row["panel"], "/plugins/x/files/10-a.conf");
        assert_eq!(row["cells"][0]["text"], "10-a.conf");
        assert_eq!(row["cells"][0]["sub"], "/etc/x.d/");
        assert_eq!(row["cells"][1]["text"], "2");
        assert_eq!(row["cells"][1]["sub"], "options");
        assert_eq!(row["cells"][2]["actions"][0]["icon"], "square-pen");
        assert_eq!(body["drawer_icon"], "square-pen");
    }

    #[test]
    fn an_address_names_a_file_of_the_set_or_none() {
        assert_eq!(SET.path("main").as_deref(), Some("/etc/x.conf"));
        assert_eq!(SET.path("new").as_deref(), Some(""));
        assert_eq!(SET.path("10-a.conf").as_deref(), Some("/etc/x.d/10-a.conf"));
        for bad in ["10-a.nft", "../passwd.conf", "A.conf", ".conf", "a/b.conf"] {
            assert_eq!(SET.path(bad), None, "{bad}");
        }
        assert_eq!(SET.href("/etc/x.conf"), "/plugins/x/files/main");
    }

    // A saved file is staged by name under the set's folder; a name no file
    // of the set could have, or one already taken, is refused in the editor.
    #[test]
    fn a_save_stages_the_file_or_says_why_not() {
        let files = [json!({"path":"/etc/x.d/taken.conf"})];
        let (_, command) = SET
            .save(
                &files,
                "new",
                &Form::parse("filename=20-b&content=a&expected=v"),
            )
            .unwrap();
        let command = command.expect("staged");
        assert_eq!(command.name, "config-file-stage");
        assert_eq!(command.args["path"], "/etc/x.d/20-b.conf");
        for (body, refusal) in [
            ("filename=Bad&content=a", "Use lowercase"),
            ("filename=taken&content=a", "already exists"),
        ] {
            let (editor, command) = SET.save(&files, "new", &Form::parse(body)).unwrap();
            assert!(command.is_none());
            let editor = serde_json::to_value(editor).unwrap().to_string();
            assert!(editor.contains(refusal), "{editor}");
        }
    }
}
