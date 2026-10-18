//! Independently authored oracle assertions. No synthesizer or transformation
//! code is called here. These assertions alone do not qualify a release.
use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case", deny_unknown_fields)]
pub enum Assertion {
    ValueEqual { before: String, after: String },
    UnchangedOutside { paths: Vec<String> },
    Absent { path: String },
}

pub fn verify(before: &Value, after: &Value, assertions: &[Assertion]) -> Result<(), String> {
    if assertions.is_empty() {
        return Err("no independent assertions supplied".into());
    }
    for assertion in assertions {
        match assertion {
            Assertion::ValueEqual {
                before: source,
                after: target,
            } => {
                validate_pointer(source)?;
                validate_pointer(target)?;
                let left = before.pointer(source);
                let right = after.pointer(target);
                if left.is_none() || right.is_none() || left != right {
                    return Err("required value equality failed".into());
                }
            }
            Assertion::Absent { path } => {
                validate_pointer(path)?;
                if after.pointer(path).is_some() {
                    return Err("required absent path is present".into());
                }
            }
            Assertion::UnchangedOutside { paths } => {
                let mut left = before.clone();
                let mut right = after.clone();
                for path in paths {
                    remove_object_member(&mut left, path)?;
                    remove_object_member(&mut right, path)?;
                }
                if left != right {
                    return Err("unrelated fields changed".into());
                }
            }
        }
    }
    Ok(())
}

fn validate_pointer(pointer: &str) -> Result<(), String> {
    if pointer.len() > 1024 || (!pointer.is_empty() && !pointer.starts_with('/')) {
        return Err("invalid JSON Pointer".into());
    }
    let mut bytes = pointer.bytes();
    while let Some(byte) = bytes.next() {
        if byte == b'~' && !matches!(bytes.next(), Some(b'0' | b'1')) {
            return Err("invalid JSON Pointer escape".into());
        }
    }
    Ok(())
}

fn remove_object_member(value: &mut Value, pointer: &str) -> Result<(), String> {
    validate_pointer(pointer)?;
    let (parent, raw_key) = pointer.rsplit_once('/').ok_or("invalid JSON Pointer")?;
    let key = raw_key.replace("~1", "/").replace("~0", "~");
    if let Some(parent) = value.pointer_mut(parent) {
        // Week 1 invariant exclusions are object members only. Do not hide
        // array reordering or permit excluding the entire document.
        let object = parent
            .as_object_mut()
            .ok_or("only object-member exclusions supported")?;
        object.remove(&key);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn missing_null_and_large_identifiers_are_distinct() {
        let assertion = vec![Assertion::ValueEqual {
            before: "/id".into(),
            after: "/id".into(),
        }];
        assert!(verify(&json!({}), &json!({}), &assertion).is_err());
        assert!(verify(&json!({"id":null}), &json!({}), &assertion).is_err());
        let a: Value = serde_json::from_str(r#"{"id":9007199254740993}"#).unwrap();
        let b: Value = serde_json::from_str(r#"{"id":9007199254740992}"#).unwrap();
        assert!(verify(&a, &b, &assertion).is_err());
        assert!(verify(&a, &a, &assertion).is_ok());
    }

    #[test]
    fn exclusions_do_not_hide_unrelated_changes_or_null() {
        let assertions = vec![Assertion::UnchangedOutside {
            paths: vec!["/from".into(), "/to".into()],
        }];
        assert!(
            verify(
                &json!({"from":"x","id":"1"}),
                &json!({"to":"x","id":"2"}),
                &assertions
            )
            .is_err()
        );
        assert!(
            verify(
                &json!({"from":"x","note":null}),
                &json!({"to":"x"}),
                &assertions
            )
            .is_err()
        );
        assert!(
            verify(
                &json!({"from":"x","id":"1"}),
                &json!({"to":"x","id":"1"}),
                &assertions
            )
            .is_ok()
        );
    }

    #[test]
    fn pointers_are_escaped_and_arrays_are_not_reordered() {
        let assertions = vec![Assertion::UnchangedOutside {
            paths: vec!["/a~1b/~0name".into()],
        }];
        assert!(
            verify(
                &json!({"a/b":{"~name":1}}),
                &json!({"a/b":{"~name":2}}),
                &assertions
            )
            .is_ok()
        );
        let bad = vec![Assertion::UnchangedOutside {
            paths: vec!["/items/0".into()],
        }];
        assert!(verify(&json!({"items":[1]}), &json!({"items":[2]}), &bad).is_err());
        let invalid = vec![Assertion::Absent {
            path: "/bad~2/field".into(),
        }];
        assert!(verify(&json!({}), &json!({}), &invalid).is_err());
    }
}
