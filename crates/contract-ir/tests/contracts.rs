use contract_ir::{
    detect::{Context, detect},
    schema::{canonical_value, decimal, normalize},
    wire::decode,
};
use serde_json::{Value, json};

fn context() -> Context {
    Context {
        http_status: 200,
        content_type: "application/json".into(),
        content_encoding: "identity".into(),
        capture_quality: "complete".into(),
        sample_probability: 1.0,
        business_decline: false,
        invariant_violation: false,
    }
}

#[test]
fn normalization_preserves_truth_and_normalizes_sets() {
    let left = json!({"type":"object","properties":{"a":{"type":["string","null"]},"b":{"enum":[2,1]}},"required":["b","a"]});
    let right = json!({"required":["a","b"],"properties":{"b":{"enum":[1.0,2e0]},"a":{"type":["null","string"]}},"type":"object","additionalProperties":true});
    assert_eq!(
        serde_json::to_value(normalize(&left, "").unwrap().root).unwrap(),
        serde_json::to_value(normalize(&right, "").unwrap().root).unwrap()
    );
    let absent = detect(
        &normalize(&left, "").unwrap().root,
        &context(),
        r#"{"b":1}"#,
    )
    .unwrap();
    assert_eq!(absent.changes[0].code, "missing_required");
    assert_eq!(
        detect(
            &normalize(&left, "").unwrap().root,
            &context(),
            r#"{"a":null,"b":1}"#
        )
        .unwrap()
        .class,
        "healthy"
    );
}

#[test]
fn exact_numbers_and_constraints_do_not_round() {
    let schema: Value = serde_json::from_str(
        r#"{"type":"integer","minimum":9007199254740993,"maximum":9007199254740994}"#,
    )
    .unwrap();
    let node = normalize(&schema, "").unwrap().root;
    assert_eq!(
        detect(&node, &context(), "9007199254740992").unwrap().class,
        "structural"
    );
    assert_eq!(
        detect(&node, &context(), "9007199254740993.0")
            .unwrap()
            .class,
        "healthy"
    );
    assert_eq!(
        detect(&node, &context(), "9007199254740995").unwrap().class,
        "structural"
    );
    for (input, expected) in [
        ("1e2", "100"),
        ("1.2300", "1.23"),
        ("-0.00", "0"),
        ("0.0010", "0.001"),
        ("-12e-2", "-0.12"),
    ] {
        assert_eq!(decimal(input).unwrap(), expected);
    }
    assert!(decimal("1e100000").is_err());
    let schema = json!({"const":null});
    assert_eq!(
        detect(&normalize(&schema, "").unwrap().root, &context(), "null")
            .unwrap()
            .class,
        "healthy"
    );
    assert_ne!(
        canonical_value(&json!(9007199254740993u64)).unwrap(),
        canonical_value(&json!(9007199254740992u64)).unwrap()
    );
}

#[test]
fn refs_openapi_and_nullable_have_visible_boundaries() {
    let openapi = json!({"openapi":"3.0.4","components":{"schemas":{"X":{"type":"string","nullable":true},"Y":{"$ref":"#/components/schemas/X"}}}});
    let ir = normalize(&openapi, "/components/schemas/Y").unwrap();
    assert_eq!(ir.diagnostics, vec!["openapi_3_0_nullable_normalized"]);
    assert_eq!(
        detect(&ir.root, &context(), "null").unwrap().class,
        "healthy"
    );
    assert!(normalize(&openapi, "").is_err());
    let legacy_bounds = json!({"openapi":"3.0.4","components":{"schemas":{"X":{"type":"number","minimum":0,"exclusiveMinimum":true}}}});
    assert!(normalize(&legacy_bounds, "/components/schemas/X").is_err());
    for schema in [
        json!({"$ref":"https://example.invalid/schema"}),
        json!({"$ref":"#/missing"}),
        json!({"$ref":"#"}),
        json!({"pattern":"(a+)+"}),
        json!({"format":"date-time"}),
        json!({"readOnly":true}),
        json!({"allOf":[{}]}),
        json!({"$id":"nested"}),
        json!({"type":"unknown"}),
    ] {
        assert!(normalize(&schema, "").is_err());
    }
    let mut deep = json!({});
    for _ in 0..40 {
        deep = json!({"items":deep});
    }
    assert!(normalize(&deep, "").is_err());
}

#[test]
fn raw_decode_rejects_duplicate_reserved_and_trailing_data() {
    for input in [
        r#"{"x":1,"x":2}"#,
        r#"{"a":{"x":1,"x":2}}"#,
        r#"{"$serde_json::private::Number":"1"}"#,
        "{} {}",
        "{",
        "[1,]",
    ] {
        assert!(decode(input.as_bytes()).is_err());
    }
    assert!(decode(&vec![b' '; 256 * 1024 + 1]).is_err());
}

#[test]
fn unions_arrays_and_redaction_remain_distinct() {
    let node = normalize(
        &json!({"type":"array","items":{"oneOf":[{"type":"string"},{"type":"null"}]}}),
        "",
    )
    .unwrap()
    .root;
    let result = detect(&node, &context(), r#"[null,"SECRET",null]"#).unwrap();
    let shape = result.shape.unwrap();
    assert_eq!(shape.items.len(), 2);
    assert!(shape.items.iter().all(|v| v.one_of_matches.len() == 1));
    assert!(!serde_json::to_string(&shape).unwrap().contains("SECRET"));
    assert!(
        detect(&node, &context(), "[]")
            .unwrap()
            .shape
            .unwrap()
            .items
            .is_empty()
    );
    let ambiguous = normalize(&json!({"oneOf":[{"type":"integer"},{"type":"number"}]}), "")
        .unwrap()
        .root;
    assert_eq!(
        detect(&ambiguous, &context(), "1").unwrap().changes[0].code,
        "one_of_mismatch"
    );
    let closed = normalize(&json!({"type":"object","additionalProperties":false}), "")
        .unwrap()
        .root;
    let report = serde_json::to_string(
        &detect(&closed, &context(), r#"{"KEY_SECRET":"VALUE_SECRET"}"#).unwrap(),
    )
    .unwrap();
    assert!(!report.contains("KEY_SECRET") && !report.contains("VALUE_SECRET"));
}

#[test]
fn operational_and_quality_failures_never_become_patches() {
    let node = normalize(&json!({"type":"object","required":["id"]}), "")
        .unwrap()
        .root;
    for status in [401, 403, 429, 500, 503] {
        let mut c = context();
        c.http_status = status;
        let result = detect(&node, &c, "<html>").unwrap();
        assert_eq!(result.class, "operational");
        assert!(!result.automatic_repair_eligible);
    }
    for quality in ["truncated", "redacted", "dropped", "unknown"] {
        let mut c = context();
        c.capture_quality = quality.into();
        assert_eq!(detect(&node, &c, "{}").unwrap().class, "unsupported");
    }
    let mut c = context();
    c.business_decline = true;
    assert_eq!(detect(&node, &c, "{}").unwrap().class, "semantic");
    let mut c = context();
    c.content_type = "text/html".into();
    assert_eq!(detect(&node, &c, "<html>").unwrap().class, "unsupported");
}
