use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};

pub const MAX_BYTES: usize = 256 * 1024;
const MAX_DEPTH: usize = 32;
const MAX_NODES: usize = 4096;
const MAX_REFS: usize = 64;

#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Kind {
    Null,
    Boolean,
    Integer,
    Number,
    String,
    Array,
    Object,
}

/// All defaults are explicit in the normalized representation. Empty `types`
/// means unconstrained; JSON Schema's type-specific keywords still apply.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Node {
    pub allowed: bool,
    pub types: BTreeSet<Kind>,
    pub properties: BTreeMap<String, Node>,
    pub required: BTreeSet<String>,
    pub additional: Additional,
    pub items: Option<Box<Node>>,
    pub any_of: Vec<Node>,
    pub one_of: Vec<Node>,
    pub values: Option<Vec<Value>>,
    pub bounds: BTreeMap<String, String>,
    pub sizes: BTreeMap<String, u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum Additional {
    Allow(bool),
    Schema(Box<Node>),
}

impl Default for Node {
    fn default() -> Self {
        Self {
            allowed: true,
            types: BTreeSet::new(),
            properties: BTreeMap::new(),
            required: BTreeSet::new(),
            additional: Additional::Allow(true),
            items: None,
            any_of: vec![],
            one_of: vec![],
            values: None,
            bounds: BTreeMap::new(),
            sizes: BTreeMap::new(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Normalized {
    pub ir_version: u32,
    pub root: Node,
    pub diagnostics: Vec<String>,
}

/// Input is JSON only. OpenAPI callers select an explicit schema JSON Pointer;
/// the complete document remains the local reference root. No I/O occurs here.
pub fn normalize(document: &Value, pointer: &str) -> Result<Normalized, String> {
    let openapi = document.get("openapi").and_then(Value::as_str);
    let legacy = match openapi {
        Some("3.0.0" | "3.0.1" | "3.0.2" | "3.0.3" | "3.0.4") => true,
        Some("3.1.0" | "3.1.1") | None => false,
        Some(_) => return Err("unsupported OpenAPI version".into()),
    };
    if openapi.is_some() && pointer.is_empty() {
        return Err("OpenAPI import requires an explicit schema pointer".into());
    }
    validate_pointer(pointer)?;
    let source = document
        .pointer(pointer)
        .ok_or("schema pointer not found")?;
    let mut loader = Loader {
        document,
        nodes: 0,
        refs: 0,
        stack: BTreeSet::new(),
        legacy,
        diagnostics: BTreeSet::new(),
    };
    let root = loader.node(source, 0)?;
    Ok(Normalized {
        ir_version: 1,
        root,
        diagnostics: loader.diagnostics.into_iter().collect(),
    })
}

struct Loader<'a> {
    document: &'a Value,
    nodes: usize,
    refs: usize,
    stack: BTreeSet<String>,
    legacy: bool,
    diagnostics: BTreeSet<String>,
}

impl Loader<'_> {
    fn node(&mut self, value: &Value, depth: usize) -> Result<Node, String> {
        self.nodes += 1;
        if depth > MAX_DEPTH || self.nodes > MAX_NODES {
            return Err("schema depth or expanded-node limit".into());
        }
        if let Some(allowed) = value.as_bool() {
            if self.legacy {
                return Err("boolean schemas require JSON Schema 2020-12".into());
            }
            return Ok(Node {
                allowed,
                ..Node::default()
            });
        }
        let obj = value
            .as_object()
            .ok_or("schema must be an object or boolean")?;
        if self.legacy
            && ["$defs", "const", "exclusiveMinimum", "exclusiveMaximum"]
                .iter()
                .any(|key| obj.contains_key(*key))
        {
            return Err("unsupported OpenAPI 3.0 keyword form".into());
        }
        for (key, value) in obj {
            match key.as_str() {
                "$schema" => {
                    if value != "https://json-schema.org/draft/2020-12/schema" || self.legacy {
                        return Err("unsupported nested schema dialect".into());
                    }
                }
                "title" | "description" | "$comment" => {
                    if !value.is_string() {
                        return Err("annotation must be a string".into());
                    }
                }
                "default" | "examples" | "example" => {
                    self.diagnostics
                        .insert("annotations_are_not_assertions".into());
                }
                "$ref"
                | "$defs"
                | "type"
                | "properties"
                | "required"
                | "additionalProperties"
                | "items"
                | "anyOf"
                | "oneOf"
                | "enum"
                | "const"
                | "minimum"
                | "maximum"
                | "exclusiveMinimum"
                | "exclusiveMaximum"
                | "minLength"
                | "maxLength"
                | "minItems"
                | "maxItems"
                | "minProperties"
                | "maxProperties" => {}
                "nullable" if self.legacy => {}
                // Regex, formats, direction annotations and semantic extensions
                // are blocked rather than silently stripped from proof input.
                _ => return Err("unsupported schema keyword (see supported-keyword matrix)".into()),
            }
        }
        if let Some(defs) = obj.get("$defs") {
            for definition in defs.as_object().ok_or("$defs must be an object")?.values() {
                self.node(definition, depth + 1)?;
            }
        }
        if let Some(reference) = obj.get("$ref") {
            if obj.keys().any(|k| {
                !matches!(
                    k.as_str(),
                    "$ref" | "$defs" | "$schema" | "title" | "description" | "$comment"
                )
            }) {
                return Err("$ref assertion siblings unsupported".into());
            }
            let reference = reference.as_str().ok_or("$ref must be a string")?;
            let pointer = reference
                .strip_prefix('#')
                .ok_or("external references disabled")?;
            validate_pointer(pointer)?;
            self.refs += 1;
            if self.refs > MAX_REFS || !self.stack.insert(reference.into()) {
                return Err("reference cycle or expansion limit".into());
            }
            let target = self
                .document
                .pointer(pointer)
                .ok_or("unresolved local reference")?;
            let result = self.node(target, depth + 1);
            self.stack.remove(reference);
            return result;
        }
        let mut node = Node::default();
        if let Some(types) = obj.get("type") {
            let types = match types {
                Value::String(_) => vec![types],
                Value::Array(types) if !self.legacy && !types.is_empty() => types.iter().collect(),
                _ => return Err("invalid type declaration".into()),
            };
            for kind in types {
                let kind = match kind.as_str() {
                    Some("null") if !self.legacy => Kind::Null,
                    Some("boolean") => Kind::Boolean,
                    Some("integer") => Kind::Integer,
                    Some("number") => Kind::Number,
                    Some("string") => Kind::String,
                    Some("array") => Kind::Array,
                    Some("object") => Kind::Object,
                    _ => return Err("invalid schema type".into()),
                };
                if !node.types.insert(kind) {
                    return Err("duplicate type".into());
                }
            }
        }
        if let Some(nullable) = obj.get("nullable") {
            let nullable = nullable.as_bool().ok_or("nullable must be boolean")?;
            if nullable {
                if node.types.is_empty()
                    || obj.contains_key("enum")
                    || obj.contains_key("const")
                    || obj.contains_key("anyOf")
                    || obj.contains_key("oneOf")
                {
                    return Err("ambiguous OpenAPI nullable composition".into());
                }
                node.types.insert(Kind::Null);
                self.diagnostics
                    .insert("openapi_3_0_nullable_normalized".into());
            }
        }
        if let Some(properties) = obj.get("properties") {
            for (key, child) in properties
                .as_object()
                .ok_or("properties must be an object")?
            {
                if key.len() > 256 {
                    return Err("property-name limit".into());
                }
                node.properties
                    .insert(key.clone(), self.node(child, depth + 1)?);
            }
        }
        if let Some(required) = obj.get("required") {
            for key in required.as_array().ok_or("required must be an array")? {
                let key = key.as_str().ok_or("required member must be a string")?;
                if key.len() > 256 || !node.required.insert(key.into()) {
                    return Err("invalid required members".into());
                }
            }
        }
        if let Some(additional) = obj.get("additionalProperties") {
            node.additional = match additional.as_bool() {
                Some(allowed) => Additional::Allow(allowed),
                None => Additional::Schema(Box::new(self.node(additional, depth + 1)?)),
            };
        }
        if let Some(items) = obj.get("items") {
            node.items = Some(Box::new(self.node(items, depth + 1)?));
        }
        for (name, target) in [("anyOf", &mut node.any_of), ("oneOf", &mut node.one_of)] {
            if let Some(branches) = obj.get(name) {
                let branches = branches.as_array().ok_or("union must be an array")?;
                if branches.is_empty() || branches.len() > 8 {
                    return Err("union branch limit".into());
                }
                for branch in branches {
                    target.push(self.node(branch, depth + 1)?);
                }
                target.sort_by_cached_key(|n| serde_json::to_string(n).unwrap());
            }
        }
        if let Some(values) = obj.get("enum") {
            let values = values.as_array().ok_or("enum must be an array")?;
            if values.is_empty() || values.len() > 128 {
                return Err("enum size limit".into());
            }
            let mut normalized: Vec<Value> = values
                .iter()
                .map(canonical_value)
                .collect::<Result<_, _>>()?;
            normalized.sort_by_cached_key(|v| serde_json::to_string(v).unwrap());
            let count = normalized.len();
            normalized.dedup();
            if normalized.len() != count {
                return Err("duplicate enum member".into());
            }
            node.values = Some(normalized);
        }
        if let Some(value) = obj.get("const") {
            let value = canonical_value(value)?;
            node.values = Some(match node.values {
                Some(values) if !values.contains(&value) => vec![],
                _ => vec![value],
            });
        }
        for name in ["minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum"] {
            if let Some(value) = obj.get(name) {
                if !value.is_number() {
                    return Err("numeric bound must be a number".into());
                }
                node.bounds
                    .insert(name.into(), decimal(&value.to_string())?);
            }
        }
        for name in [
            "minLength",
            "maxLength",
            "minItems",
            "maxItems",
            "minProperties",
            "maxProperties",
        ] {
            if let Some(value) = obj.get(name) {
                node.sizes.insert(
                    name.into(),
                    value
                        .as_u64()
                        .ok_or("size bound must be a nonnegative u64 integer")?,
                );
            }
        }
        Ok(node)
    }
}

pub fn validate_pointer(pointer: &str) -> Result<(), String> {
    if pointer.len() > 1024
        || (!pointer.is_empty() && !pointer.starts_with('/'))
        || pointer.contains('%')
    {
        return Err("unsupported JSON Pointer".into());
    }
    let mut bytes = pointer.bytes();
    while let Some(byte) = bytes.next() {
        if byte == b'~' && !matches!(bytes.next(), Some(b'0' | b'1')) {
            return Err("invalid pointer escape".into());
        }
    }
    Ok(())
}

pub fn escape(key: &str) -> String {
    key.replace('~', "~0").replace('/', "~1")
}

// Exact bounded decimal normalization; no f64 conversion. Exponent expansion is
// bounded to keep arbitrarily small/large adversarial tokens from allocating.
pub fn decimal(token: &str) -> Result<String, String> {
    if token.len() > 256 {
        return Err("numeric token limit".into());
    }
    let (mantissa, exponent) = match token.find(['e', 'E']) {
        Some(index) => (
            &token[..index],
            token[index + 1..]
                .parse::<i32>()
                .map_err(|_| "numeric exponent limit")?,
        ),
        None => (token, 0),
    };
    if exponent.unsigned_abs() > 256 {
        return Err("numeric exponent limit".into());
    }
    let negative = mantissa.starts_with('-');
    let mantissa = mantissa.trim_start_matches('-');
    let fraction = mantissa.split_once('.').map_or(0, |(_, f)| f.len()) as i32;
    let digits = mantissa.replace('.', "");
    let digits = digits.trim_start_matches('0');
    if digits.is_empty() {
        return Ok("0".into());
    }
    let point = digits.len() as i32 + exponent - fraction;
    let mut result = if point <= 0 {
        format!("0.{}{}", "0".repeat((-point) as usize), digits)
    } else if point as usize >= digits.len() {
        format!("{}{}", digits, "0".repeat(point as usize - digits.len()))
    } else {
        format!(
            "{}.{}",
            &digits[..point as usize],
            &digits[point as usize..]
        )
    };
    if result.contains('.') {
        result = result.trim_end_matches('0').trim_end_matches('.').into();
    }
    if negative {
        result.insert(0, '-');
    }
    Ok(result)
}

pub fn canonical_value(value: &Value) -> Result<Value, String> {
    Ok(match value {
        Value::Number(number) => serde_json::from_str(&decimal(&number.to_string())?)
            .map_err(|_| "invalid numeric token")?,
        Value::Array(items) => Value::Array(
            items
                .iter()
                .map(canonical_value)
                .collect::<Result<_, _>>()?,
        ),
        Value::Object(properties) => Value::Object(
            properties
                .iter()
                .map(|(k, v)| Ok((k.clone(), canonical_value(v)?)))
                .collect::<Result<_, String>>()?,
        ),
        _ => value.clone(),
    })
}

pub fn compare_decimal(left: &str, right: &str) -> std::cmp::Ordering {
    let left_negative = left.starts_with('-');
    let right_negative = right.starts_with('-');
    if left_negative != right_negative {
        return right_negative.cmp(&left_negative);
    }
    let left = left.trim_start_matches('-');
    let right = right.trim_start_matches('-');
    let (li, lf) = left.split_once('.').unwrap_or((left, ""));
    let (ri, rf) = right.split_once('.').unwrap_or((right, ""));
    let length = lf.len().max(rf.len());
    let order = li
        .len()
        .cmp(&ri.len())
        .then_with(|| li.cmp(ri))
        .then_with(|| {
            lf.bytes()
                .chain(std::iter::repeat(b'0'))
                .take(length)
                .cmp(rf.bytes().chain(std::iter::repeat(b'0')).take(length))
        });
    if left_negative {
        order.reverse()
    } else {
        order
    }
}
