use adapter_core::{Input, Output, Plan};
use executor::Runtime;

fn artifact() -> Vec<u8> {
    std::fs::read("../../target/wasm32-unknown-unknown/release/adapter_wasm.wasm")
        .expect("run make wasm before runtime tests")
}

#[test]
fn native_wasm_equivalence_and_isolation() {
    let runtime = Runtime::new(&artifact()).unwrap();
    let fixtures = [
        (
            "stripe",
            "/data/object/metadata/presentation/label",
            "/data/object/metadata/display_label",
        ),
        ("shopify", "/details/vendor", "/vendor"),
        ("netsuite", "/presentation/displayName", "/displayName"),
        (
            "generic-json",
            "/shipment/tracking/number",
            "/shipment/tracking_number",
        ),
    ];
    for (family, from, path) in fixtures {
        for case in ["field-move", "required-deletion", "baseline"] {
            let body =
                std::fs::read_to_string(format!("../../fixtures/synthetic/{family}/{case}.json"))
                    .unwrap();
            for plan in [
                Plan::Identity,
                Plan::MoveString {
                    from: from.into(),
                    path: path.into(),
                },
            ] {
                let input = Input {
                    abi_version: 1,
                    plan,
                    body: body.clone(),
                };
                let native: Output = serde_json::from_slice(&adapter_core::execute(
                    &serde_json::to_vec(&input).unwrap(),
                ))
                .unwrap();
                assert_eq!(native, runtime.run(&input).unwrap(), "{family}/{case}");
            }
        }
    }
    for body in ["{", "{\"x\":1,\"x\":2}", "{}{}"] {
        let output = runtime
            .run(&Input {
                abi_version: 1,
                plan: Plan::Identity,
                body: body.into(),
            })
            .unwrap();
        assert!(matches!(output, Output::Rejected { .. }));
    }
}

#[test]
fn sandbox_rejects_imports_and_bounds_runaway_execution() {
    assert!(Runtime::new(br#"(module (import "env" "network" (func)))"#).is_err());
    let prefix = r#"(module (memory (export "memory") 1)
      (func (export "abi_version") (result i32) i32.const 1)
      (func (export "dealloc") (param i32 i32))"#;
    let loop_alloc = format!(
        r#"{prefix} (func (export "alloc") (param i32) (result i32) (loop $loop br $loop) i32.const 0)
      (func (export "transform") (param i32 i32 i32 i32) (result i32) i32.const 0))"#
    );
    let runtime = Runtime::new(loop_alloc.as_bytes()).unwrap();
    let started = std::time::Instant::now();
    assert!(
        runtime
            .run(&Input {
                abi_version: 1,
                plan: Plan::Identity,
                body: "{}".into()
            })
            .is_err()
    );
    assert!(started.elapsed() < std::time::Duration::from_secs(1));
    let bad_pointer = format!(
        r#"{prefix} (func (export "alloc") (param i32) (result i32) i32.const -1)
      (func (export "transform") (param i32 i32 i32 i32) (result i32) i32.const -1))"#
    );
    let runtime = Runtime::new(bad_pointer.as_bytes()).unwrap();
    assert!(
        runtime
            .run(&Input {
                abi_version: 1,
                plan: Plan::Identity,
                body: "{}".into()
            })
            .is_err()
    );
    let too_much_memory = br#"(module (memory (export "memory") 513))"#;
    let runtime = Runtime::new(too_much_memory).unwrap();
    assert!(
        runtime
            .run(&Input {
                abi_version: 1,
                plan: Plan::Identity,
                body: "{}".into()
            })
            .is_err()
    );
}
