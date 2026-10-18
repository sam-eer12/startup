//! Versioned wire vocabulary only. A parsed operation is not a validated plan;
//! typing, authorization and execution are deliberately deferred to Week 4.
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "op", rename_all = "snake_case", deny_unknown_fields)]
pub enum Operation {
    Copy {
        from: String,
        path: String,
        source_type: String,
        target_type: String,
        on_missing: Reject,
        on_conflict: Reject,
    },
    Move {
        from: String,
        path: String,
        source_type: String,
        target_type: String,
        on_missing: Reject,
        on_conflict: Reject,
    },
    Remove {
        path: String,
        authorization_ref: String,
    },
    Assert {
        path: String,
        expected_type: String,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Reject {
    Reject,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn unknown_and_unsafe_operation_fields_fail_closed() {
        for body in [
            r#"{"op":"execute","shell":"echo unsafe"}"#,
            r#"{"op":"remove","path":"/id"}"#,
            r#"{"op":"assert","path":"/id","expected_type":"string","ignored":true}"#,
        ] {
            assert!(serde_json::from_str::<Operation>(body).is_err());
        }
    }
}
