//! Validate the raw JSON tree before serde_json's exact-number Value decoding.
use crate::schema::MAX_BYTES;
use serde::{Deserialize, de};
use serde_json::{Value, value::RawValue};
use std::{collections::BTreeSet, fmt};

pub fn decode(bytes: &[u8]) -> Result<Value, String> {
    if bytes.len() > MAX_BYTES {
        return Err("JSON byte limit".into());
    }
    let raw: Box<RawValue> = serde_json::from_slice(bytes).map_err(|_| "invalid JSON")?;
    validate(&raw, 0, &mut 0)?;
    serde_json::from_slice(bytes).map_err(|_| "invalid JSON".into())
}

fn validate(raw: &RawValue, depth: usize, nodes: &mut usize) -> Result<(), String> {
    *nodes += 1;
    if depth > 48 || *nodes > 10_000 {
        return Err("JSON depth or node limit".into());
    }
    match raw.get().as_bytes().first() {
        Some(b'{') => {
            let Members(members) =
                serde_json::from_str(raw.get()).map_err(|_| "duplicate or reserved key")?;
            for (_, child) in members {
                validate(&child, depth + 1, nodes)?;
            }
        }
        Some(b'[') => {
            let children: Vec<Box<RawValue>> =
                serde_json::from_str(raw.get()).map_err(|_| "invalid array")?;
            for child in children {
                validate(&child, depth + 1, nodes)?;
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
                f.write_str("unique object keys")
            }
            fn visit_map<A: de::MapAccess<'de>>(self, mut map: A) -> Result<Members, A::Error> {
                let mut seen = BTreeSet::new();
                let mut members = vec![];
                while let Some(key) = map.next_key::<String>()? {
                    if key == "$serde_json::private::Number" || !seen.insert(key.clone()) {
                        return Err(de::Error::custom("duplicate or reserved key"));
                    }
                    members.push((key, map.next_value()?));
                }
                Ok(Members(members))
            }
        }
        deserializer.deserialize_map(Visitor)
    }
}
