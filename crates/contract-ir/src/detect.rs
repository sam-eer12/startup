use crate::schema::{Additional, Kind, Node, canonical_value, compare_decimal, escape};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::BTreeMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Context {
    pub http_status: u16,
    pub content_type: String,
    pub content_encoding: String,
    pub capture_quality: String,
    pub sample_probability: f64,
    /// Explicit local fixture feedback, not inferred from an arbitrary payload.
    pub business_decline: bool,
    pub invariant_violation: bool,
}

#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
pub struct Change {
    pub code: String,
    pub path: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Detection {
    pub class: String,
    pub reason: String,
    pub changes: Vec<Change>,
    pub shape: Option<Shape>,
    pub automatic_repair_eligible: bool,
    pub confidence: String,
    pub sample_probability: f64,
    pub semantic_verification: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Shape {
    pub kind: String,
    pub properties: BTreeMap<String, Shape>,
    pub missing_required: Vec<String>,
    pub items: Vec<Shape>,
    pub unknown_members: usize,
    pub any_of_matches: Vec<usize>,
    pub one_of_matches: Vec<usize>,
}

pub fn detect(node: &Node, context: &Context, body: &str) -> Result<Detection, String> {
    if !(100..=599).contains(&context.http_status)
        || !context.sample_probability.is_finite()
        || context.sample_probability <= 0.0
        || context.sample_probability > 1.0
        || !["complete", "truncated", "redacted", "dropped", "unknown"]
            .contains(&context.capture_quality.as_str())
    {
        return Err("invalid observation context".into());
    }
    let mut result = Detection {
        class: "unsupported".into(),
        reason: "incomplete_capture".into(),
        changes: vec![],
        shape: None,
        automatic_repair_eligible: false,
        confidence: "insufficient_evidence".into(),
        sample_probability: context.sample_probability,
        semantic_verification: "unavailable_without_explicit_invariants".into(),
    };
    // Status and declared business feedback take precedence over schema mismatch.
    if context.http_status >= 300 {
        result.class = "operational".into();
        result.reason = match context.http_status {
            401 | 403 => "authentication",
            429 => "throttling",
            500..=599 => "availability",
            _ => "http_status",
        }
        .into();
        return Ok(result);
    }
    if context.business_decline || context.invariant_violation {
        result.class = "semantic".into();
        result.reason = if context.invariant_violation {
            "explicit_invariant_violation"
        } else {
            "business_decline"
        }
        .into();
        result.semantic_verification = "explicit_local_fixture_feedback".into();
        return Ok(result);
    }
    if context.capture_quality != "complete" {
        return Ok(result);
    }
    if context.http_status == 206 {
        result.reason = "partial_http_response".into();
        return Ok(result);
    }
    if context.content_encoding != "identity" && !context.content_encoding.is_empty() {
        result.reason = "unsupported_encoding".into();
        return Ok(result);
    }
    if context.content_type.split(';').next().unwrap_or("").trim() != "application/json" {
        result.reason = "unsupported_content".into();
        return Ok(result);
    }
    let value = match crate::wire::decode(body.as_bytes()).and_then(|v| canonical_value(&v)) {
        Ok(value) => value,
        Err(_) => {
            result.reason = "invalid_or_unbounded_json".into();
            return Ok(result);
        }
    };
    let mut budget = 50_000;
    let mut changes = check(node, &value, "", &mut budget)?;
    changes.sort();
    changes.dedup();
    if changes.len() > 64 {
        return Err("change limit".into());
    }
    result.class = if changes.is_empty() {
        "healthy"
    } else {
        "structural"
    }
    .into();
    result.reason = if changes.is_empty() {
        "contract_satisfied"
    } else {
        "consumer_contract_mismatch"
    }
    .into();
    result.confidence = "single_complete_observation".into();
    result.changes = changes;
    // This representation contains only schema-approved property names and types.
    // Unknown keys and all scalar values remain local and are not returned.
    result.shape = Some(shape(node, &value, &mut budget)?);
    Ok(result)
}

fn kind(value: &Value) -> Kind {
    match value {
        Value::Null => Kind::Null,
        Value::Bool(_) => Kind::Boolean,
        Value::String(_) => Kind::String,
        Value::Number(n) if !n.to_string().contains('.') => Kind::Integer,
        Value::Number(_) => Kind::Number,
        Value::Array(_) => Kind::Array,
        Value::Object(_) => Kind::Object,
    }
}

fn check(
    node: &Node,
    value: &Value,
    path: &str,
    budget: &mut usize,
) -> Result<Vec<Change>, String> {
    *budget = budget.checked_sub(1).ok_or("validation work limit")?;
    let mut changes = vec![];
    let mut add = |code: &str| {
        changes.push(Change {
            code: code.into(),
            path: path.into(),
        })
    };
    if !node.allowed {
        add("forbidden_value");
        return Ok(changes);
    }
    let actual = kind(value);
    if !node.types.is_empty()
        && !node.types.contains(&actual)
        && !(actual == Kind::Integer && node.types.contains(&Kind::Number))
    {
        add(if actual == Kind::Null {
            "nullable_mismatch"
        } else {
            "type_mismatch"
        });
        return Ok(changes);
    }
    if let Some(values) = &node.values
        && !values.contains(value)
    {
        add("enum_or_const_mismatch");
    }
    for (branches, exclusive) in [(&node.any_of, false), (&node.one_of, true)] {
        if !branches.is_empty() {
            let mut matches = 0;
            for branch in branches {
                if check(branch, value, path, budget)?.is_empty() {
                    matches += 1;
                }
            }
            if matches == 0 || (exclusive && matches != 1) {
                add(if exclusive {
                    "one_of_mismatch"
                } else {
                    "any_of_mismatch"
                });
            }
        }
    }
    if value.is_number() {
        let number = value.to_string();
        for (name, bound) in &node.bounds {
            let order = compare_decimal(&number, bound);
            let valid = match name.as_str() {
                "minimum" => !order.is_lt(),
                "maximum" => !order.is_gt(),
                "exclusiveMinimum" => order.is_gt(),
                "exclusiveMaximum" => order.is_lt(),
                _ => unreachable!(),
            };
            if !valid {
                add("numeric_constraint_mismatch");
            }
        }
    }
    let size = match value {
        Value::String(v) => Some((v.chars().count(), "minLength", "maxLength")),
        Value::Array(v) => Some((v.len(), "minItems", "maxItems")),
        Value::Object(v) => Some((v.len(), "minProperties", "maxProperties")),
        _ => None,
    };
    if let Some((size, min, max)) = size
        && (node.sizes.get(min).is_some_and(|v| (size as u64) < *v)
            || node.sizes.get(max).is_some_and(|v| (size as u64) > *v))
    {
        add("size_constraint_mismatch");
    }
    if let Some(object) = value.as_object() {
        for key in &node.required {
            if !object.contains_key(key) {
                changes.push(Change {
                    code: "missing_required".into(),
                    path: format!("{path}/{}", escape(key)),
                });
            }
        }
        for (key, child) in object {
            if let Some(schema) = node.properties.get(key) {
                changes.extend(check(
                    schema,
                    child,
                    &format!("{path}/{}", escape(key)),
                    budget,
                )?);
            } else {
                // Use a fixed path for unapproved names, including names inside
                // an additionalProperties schema; no unknown keys enter reports.
                let unknown_path = format!("{path}/[unapproved]");
                match &node.additional {
                    Additional::Allow(false) => changes.push(Change {
                        code: "added_property".into(),
                        path: unknown_path,
                    }),
                    Additional::Schema(schema) => {
                        changes.extend(check(schema, child, &unknown_path, budget)?)
                    }
                    _ => {}
                }
            }
        }
    }
    if let Some(array) = value.as_array()
        && let Some(items) = &node.items
    {
        for child in array {
            changes.extend(check(items, child, &format!("{path}/*"), budget)?);
        }
    }
    if changes.len() > 64 {
        return Err("change limit".into());
    }
    Ok(changes)
}

fn shape(node: &Node, value: &Value, budget: &mut usize) -> Result<Shape, String> {
    *budget = budget.checked_sub(1).ok_or("shape work limit")?;
    let mut result = Shape {
        kind: format!("{:?}", kind(value)).to_lowercase(),
        properties: BTreeMap::new(),
        missing_required: vec![],
        items: vec![],
        unknown_members: 0,
        any_of_matches: vec![],
        one_of_matches: vec![],
    };
    if let Some(object) = value.as_object() {
        for (key, child) in object {
            if let Some(schema) = node.properties.get(key) {
                result
                    .properties
                    .insert(key.clone(), shape(schema, child, budget)?);
            } else {
                result.unknown_members += 1;
            }
        }
        result.missing_required = node
            .required
            .iter()
            .filter(|k| !object.contains_key(*k))
            .cloned()
            .collect();
    }
    if let Some(array) = value.as_array() {
        let unconstrained = Node::default();
        let schema = node.items.as_deref().unwrap_or(&unconstrained);
        let mut unique = BTreeMap::new();
        for item in array {
            let item = shape(schema, item, budget)?;
            unique.insert(serde_json::to_string(&item).unwrap(), item);
        }
        result.items = unique.into_values().collect();
    }
    for (branches, matches) in [
        (&node.any_of, &mut result.any_of_matches),
        (&node.one_of, &mut result.one_of_matches),
    ] {
        for (index, branch) in branches.iter().enumerate() {
            if check(branch, value, "", budget)?.is_empty() {
                matches.push(index);
            }
        }
    }
    Ok(result)
}
