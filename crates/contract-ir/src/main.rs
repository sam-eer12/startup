//! One bounded offline operation per process. Errors never echo source/payload.
use contract_ir::{
    detect::{Context, detect},
    schema::{MAX_BYTES, normalize},
    wire::decode,
};
use serde::Deserialize;
use serde_json::{Value, json};
use std::io::{self, Read};

#[derive(Deserialize)]
#[serde(tag = "operation", rename_all = "snake_case", deny_unknown_fields)]
enum Request {
    Normalize {
        document: Value,
        pointer: String,
    },
    Detect {
        document: Value,
        pointer: String,
        context: Context,
        body: String,
    },
}

fn run(bytes: &[u8]) -> Result<Value, String> {
    let request: Request =
        serde_json::from_value(decode(bytes)?).map_err(|_| "invalid operation envelope")?;
    match request {
        Request::Normalize { document, pointer } => {
            Ok(json!({"normalized": normalize(&document, &pointer)?}))
        }
        Request::Detect {
            document,
            pointer,
            context,
            body,
        } => {
            let contract = normalize(&document, &pointer)?;
            Ok(json!({"detection": detect(&contract.root, &context, &body)?}))
        }
    }
}

fn main() {
    let mut bytes = vec![];
    let result = io::stdin()
        .take(MAX_BYTES as u64 + 1)
        .read_to_end(&mut bytes)
        .map_err(|_| "input read failed".to_string())
        .and_then(|_| run(&bytes));
    println!(
        "{}",
        match result {
            Ok(value) => json!({"ok": true, "result": value}),
            Err(error) => json!({"ok": false, "error": error}),
        }
    );
}
