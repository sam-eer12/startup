use serde::Deserialize;
use serde_json::Value;
use std::{env, fs, path::Path, process};
use verifier::{Assertion, verify};

#[derive(Deserialize)]
struct Manifest {
    cases: Vec<Case>,
}
#[derive(Deserialize)]
struct Case {
    case_id: String,
    input: String,
    expected_output: Option<String>,
    assertions: Vec<Assertion>,
}

fn main() {
    if let Err(error) = run() {
        eprintln!("oracle: {error}");
        process::exit(1);
    }
}

fn read(root: &Path, path: &str) -> Result<Value, Box<dyn std::error::Error>> {
    Ok(serde_json::from_slice(&fs::read(root.join(path))?)?)
}

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let root_arg = env::args().nth(1).unwrap_or_else(|| ".".into());
    let root = Path::new(&root_arg);
    let manifest: Manifest =
        serde_json::from_value(read(root, "fixtures/manifests/corpus-v1.json")?)?;
    let mut checked = 0;
    for case in manifest.cases {
        if let Some(output) = case.expected_output {
            let before = read(root, &case.input)?;
            let after = read(root, &output)?;
            verify(&before, &after, &case.assertions)
                .map_err(|reason| format!("{}: {reason}", case.case_id))?;
            checked += 1;
        }
    }
    if checked < 4 {
        return Err("missing connector golden cases".into());
    }
    println!(
        "independent oracle: {checked} synthetic golden cases passed (no release eligibility)"
    );
    Ok(())
}
