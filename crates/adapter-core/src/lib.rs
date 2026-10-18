//! Deliberately narrow Week 2 spike, not the Week 4 compiler or schema validator.
use serde::{Deserialize, Serialize, de};
use serde_json::{Value, value::RawValue};
use std::{collections::BTreeSet, fmt};

pub const MAX_BODY: usize = 256 * 1024;
pub const MAX_ENVELOPE: usize = 2 * 1024 * 1024;

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case", deny_unknown_fields)]
pub enum Plan {
    Identity,
    /// Existing object parents, string values, no overlap, no array edits.
    MoveString {
        from: String,
        path: String,
    },
}

#[derive(Debug, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Input {
    pub abi_version: u32,
    pub plan: Plan,
    pub body: String,
}

#[derive(Debug, Serialize, Deserialize, PartialEq)]
#[serde(tag = "status", rename_all = "snake_case", deny_unknown_fields)]
pub enum Output {
    Unchanged { body: String },
    Transformed { body: String },
    Rejected { code: String },
}

impl Output {
    pub fn body(&self) -> Option<&str> {
        match self {
            Self::Unchanged { body } | Self::Transformed { body } => Some(body),
            Self::Rejected { .. } => None,
        }
    }
}

pub fn transform(input: &Input) -> Result<Output, &'static str> {
    if input.abi_version != 1 {
        return Err("unsupported_abi");
    }
    validate_json(input.body.as_bytes())?;
    if matches!(input.plan, Plan::Identity) {
        return Ok(Output::Unchanged {
            body: input.body.clone(),
        });
    }
    let Plan::MoveString { from, path } = &input.plan else {
        unreachable!()
    };
    let (source_parent, source_key) = member(from)?;
    let (target_parent, target_key) = member(path)?;
    if from == path
        || from.starts_with(&format!("{path}/"))
        || path.starts_with(&format!("{from}/"))
    {
        return Err("overlapping_paths");
    }
    let mut value: Value = serde_json::from_str(&input.body).map_err(|_| "invalid_json")?;
    let source = value
        .pointer(source_parent)
        .and_then(Value::as_object)
        .and_then(|v| v.get(&source_key))
        .ok_or("missing_source")?;
    if !source.is_string() {
        return Err("source_type");
    }
    let target = value
        .pointer(target_parent)
        .and_then(Value::as_object)
        .ok_or("missing_target_parent")?;
    if target.contains_key(&target_key) {
        return Err("destination_conflict");
    }
    let moved = value
        .pointer_mut(source_parent)
        .and_then(Value::as_object_mut)
        .unwrap()
        .remove(&source_key)
        .unwrap();
    value
        .pointer_mut(target_parent)
        .and_then(Value::as_object_mut)
        .ok_or("invalid_target_parent")?
        .insert(target_key, moved);
    let body = serde_json::to_string(&value).map_err(|_| "serialization_failed")?;
    if body.len() > MAX_BODY {
        return Err("body_too_large");
    }
    Ok(Output::Transformed { body })
}

fn member(pointer: &str) -> Result<(&str, String), &'static str> {
    if pointer.len() > 1024 || !pointer.starts_with('/') {
        return Err("invalid_pointer");
    }
    let mut chars = pointer.chars();
    while let Some(c) = chars.next() {
        if c == '~' && !matches!(chars.next(), Some('0' | '1')) {
            return Err("invalid_pointer");
        }
    }
    let (parent, key) = pointer.rsplit_once('/').ok_or("invalid_pointer")?;
    Ok((parent, key.replace("~1", "/").replace("~0", "~")))
}

/// Preflight raw values so duplicate names are rejected before Value can erase
/// them. Numbers retain their exact tokens (including integers above 2^53).
pub fn validate_json(body: &[u8]) -> Result<(), &'static str> {
    if body.len() > MAX_BODY {
        return Err("body_too_large");
    }
    let raw: Box<RawValue> = serde_json::from_slice(body).map_err(|_| "invalid_json")?;
    validate_raw(&raw, 0, &mut 0)
}

fn validate_raw(raw: &RawValue, depth: usize, nodes: &mut usize) -> Result<(), &'static str> {
    *nodes += 1;
    if depth > 64 || *nodes > 10_000 {
        return Err("json_limit");
    }
    match raw.get().as_bytes().first() {
        Some(b'{') => {
            let Members(members) =
                serde_json::from_str(raw.get()).map_err(|_| "duplicate_or_reserved_key")?;
            for (_, value) in members {
                validate_raw(&value, depth + 1, nodes)?;
            }
        }
        Some(b'[') => {
            let values: Vec<Box<RawValue>> =
                serde_json::from_str(raw.get()).map_err(|_| "invalid_json")?;
            for value in values {
                validate_raw(&value, depth + 1, nodes)?;
            }
        }
        _ => {}
    }
    Ok(())
}

struct Members(Vec<(String, Box<RawValue>)>);
impl<'de> Deserialize<'de> for Members {
    fn deserialize<D: de::Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        struct Visitor;
        impl<'de> de::Visitor<'de> for Visitor {
            type Value = Members;
            fn expecting(&self, f: &mut fmt::Formatter) -> fmt::Result {
                f.write_str("unique object members")
            }
            fn visit_map<A: de::MapAccess<'de>>(self, mut map: A) -> Result<Members, A::Error> {
                let mut seen = BTreeSet::new();
                let mut members = Vec::new();
                while let Some(key) = map.next_key::<String>()? {
                    // serde_json's arbitrary-precision sentinel is not an accepted
                    // user property; otherwise it could be interpreted as a number.
                    if key == "$serde_json::private::Number" || !seen.insert(key.clone()) {
                        return Err(de::Error::custom("duplicate or reserved object member"));
                    }
                    members.push((key, map.next_value()?));
                }
                Ok(Members(members))
            }
        }
        deserializer.deserialize_map(Visitor)
    }
}

pub fn execute(bytes: &[u8]) -> Vec<u8> {
    let result = if bytes.len() > MAX_ENVELOPE {
        Err("envelope_too_large")
    } else {
        serde_json::from_slice::<Input>(bytes)
            .map_err(|_| "invalid_envelope")
            .and_then(|input| transform(&input))
    };
    serde_json::to_vec(&result.unwrap_or_else(|code| Output::Rejected { code: code.into() }))
        .unwrap()
}

#[cfg(test)]
mod tests {
    use super::*;
    use verifier::{Assertion, verify};

    #[test]
    fn golden_and_independent_oracle() {
        let before = include_str!("../../../fixtures/synthetic/stripe/field-move.json");
        let from = "/data/object/metadata/presentation/label";
        let path = "/data/object/metadata/display_label";
        let output = transform(&Input {
            abi_version: 1,
            plan: Plan::MoveString {
                from: from.into(),
                path: path.into(),
            },
            body: before.into(),
        })
        .unwrap();
        let after: Value = serde_json::from_str(output.body().unwrap()).unwrap();
        let golden: Value = serde_json::from_str(include_str!(
            "../../../fixtures/golden/stripe/field-move.json"
        ))
        .unwrap();
        assert_eq!(after, golden);
        verify(
            &serde_json::from_str(before).unwrap(),
            &after,
            &[
                Assertion::ValueEqual {
                    before: from.into(),
                    after: path.into(),
                },
                Assertion::Absent { path: from.into() },
                Assertion::UnchangedOutside {
                    paths: vec![from.into(), path.into()],
                },
            ],
        )
        .unwrap();
    }

    #[test]
    fn exact_noop_and_fail_closed_moves() {
        let body = " {\"id\":9007199254740993,\"n\":1.000,\"x\":null} \n";
        let mut input = Input {
            abi_version: 1,
            plan: Plan::Identity,
            body: body.into(),
        };
        assert_eq!(transform(&input).unwrap().body().unwrap(), body);
        input.plan = Plan::MoveString {
            from: "/x".into(),
            path: "/y".into(),
        };
        for body in [
            r#"{"x":null}"#,
            r#"{"x":1}"#,
            r#"{}"#,
            r#"{"x":"a","y":null}"#,
            r#"{"x":"a","x":"b"}"#,
        ] {
            input.body = body.into();
            assert!(transform(&input).is_err());
        }
        input.body = r#"{"x":"a","id":9007199254740993,"n":1.000}"#.into();
        let result = transform(&input).unwrap();
        assert!(result.body().unwrap().contains("9007199254740993"));
        assert!(result.body().unwrap().contains("1.000"));
    }

    #[test]
    fn parser_limits_and_escaped_members() {
        for body in [
            b"{}{}".as_slice(),
            b"\xff",
            b"{\"x\":1,\"x\":2}",
            b"{\"$serde_json::private::Number\":\"1\"}",
        ] {
            assert!(validate_json(body).is_err());
        }
        assert!(
            validate_json(format!("{}0{}", "[".repeat(65), "]".repeat(65)).as_bytes()).is_err()
        );
        assert!(validate_json(format!("[{}0]", "0,".repeat(10_000)).as_bytes()).is_err());
        assert!(validate_json(&vec![b' '; MAX_BODY + 1]).is_err());
        let output = transform(&Input {
            abi_version: 1,
            plan: Plan::MoveString {
                from: "/a~1b/~0x".into(),
                path: "/out".into(),
            },
            body: r#"{"a/b":{"~x":"yes"}}"#.into(),
        })
        .unwrap();
        assert_eq!(
            serde_json::from_str::<Value>(output.body().unwrap()).unwrap()["out"],
            "yes"
        );
    }

    #[test]
    fn expanded_output_cannot_exceed_body_budget() {
        let base = r#"{"x":"a","padding":""}"#;
        let body = base.replace(
            "\"padding\":\"\"",
            &format!("\"padding\":\"{}\"", "p".repeat(MAX_BODY - base.len())),
        );
        let input = Input {
            abi_version: 1,
            plan: Plan::MoveString {
                from: "/x".into(),
                path: "/longer_destination".into(),
            },
            body,
        };
        assert_eq!(input.body.len(), MAX_BODY);
        assert_eq!(transform(&input), Err("body_too_large"));
        assert!(input.body.starts_with("{\"x\":\"a\""));
    }
}
