// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

use std::collections::BTreeMap;

/// Errors is what a submission got wrong, addressed to the controls that carry
/// the offending values: a message per field, and a message per item of a list.
/// The shell reads the annotations back off the re-rendered tree, so a form that
/// reports one is a 422 and its write is blocked. The first message on a field
/// wins: the earliest check is the one the operator meets first.
#[derive(Debug, Default, Clone)]
pub struct Errors {
    fields: BTreeMap<String, String>,
    lists: BTreeMap<String, BTreeMap<String, String>>,
}

impl Errors {
    /// field records a message against a field.
    pub fn field(&mut self, name: &str, message: &str) {
        self.fields
            .entry(name.to_string())
            .or_insert_with(|| message.to_string());
    }

    /// check records a message against a field when its value is not one the
    /// daemon accepts.
    pub fn check(&mut self, name: &str, ok: bool, message: &str) {
        if !ok {
            self.field(name, message);
        }
    }

    /// items validates one list and records a message under the index of each
    /// item that failed — which is how a repeating control says which row is
    /// wrong rather than reddening the whole list.
    pub fn items<F>(&mut self, name: &str, values: &[String], check: F)
    where
        F: Fn(&str) -> Result<(), &'static str>,
    {
        for (index, value) in values.iter().enumerate() {
            if let Err(message) = check(value) {
                self.lists
                    .entry(name.to_string())
                    .or_default()
                    .insert(index.to_string(), message.to_string());
            }
        }
    }

    /// get is the message for one field, or "" — what a field widget carries.
    pub fn get(&self, name: &str) -> &str {
        self.fields.get(name).map(String::as_str).unwrap_or("")
    }

    /// list is the per-item messages for one list, keyed by index as a string.
    pub fn list(&self, name: &str) -> BTreeMap<String, String> {
        self.lists.get(name).cloned().unwrap_or_default()
    }

    pub fn is_empty(&self) -> bool {
        self.fields.is_empty() && self.lists.is_empty()
    }

    /// merge folds another set of refusals in. One submission may be validated
    /// by more than one owner — a zone panel writes its zone and the crossings
    /// out of it, which are different section types with different rules — and
    /// the operator is owed every refusal at once rather than one per attempt.
    pub fn merge(&mut self, other: Errors) {
        for (name, message) in other.fields {
            self.fields.entry(name).or_insert(message);
        }
        for (name, items) in other.lists {
            let list = self.lists.entry(name).or_default();
            for (index, message) in items {
                list.entry(index).or_insert(message);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::Errors;

    #[test]
    fn the_first_message_on_a_field_wins_and_a_list_item_is_addressed_by_index() {
        let mut errors = Errors::default();
        errors.check("ssid", true, "never recorded");
        errors.field("ssid", "Give the network a name.");
        errors.field("ssid", "A later message.");
        errors.items("dns", &["1.1.1.1".into(), "nope".into()], |v| {
            if v == "nope" {
                Err("Not an address.")
            } else {
                Ok(())
            }
        });
        assert_eq!(errors.get("ssid"), "Give the network a name.");
        assert_eq!(errors.get("other"), "");
        assert_eq!(
            errors.list("dns").get("1").map(String::as_str),
            Some("Not an address.")
        );
        assert!(!errors.is_empty());
    }
}
