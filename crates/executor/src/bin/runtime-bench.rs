//! Local diagnostic microbenchmark. Does not measure Envoy/transport latency.
use adapter_core::{Input, Plan};
use anyhow::Result;
use executor::Runtime;
use serde_json::json;
use sha2::{Digest, Sha256};
use std::{
    sync::{Arc, Barrier},
    time::Instant,
};

fn main() -> Result<()> {
    let path = std::env::args()
        .nth(1)
        .unwrap_or_else(|| "target/wasm32-unknown-unknown/release/adapter_wasm.wasm".into());
    let wasm = std::fs::read(path)?;
    let compile = Instant::now();
    let runtime = Arc::new(Runtime::new(&wasm)?);
    let compile_ms = compile.elapsed().as_secs_f64() * 1000.0;
    let mut results = Vec::new();
    for size in [32 * 1024, 256 * 1024] {
        let base =
            r#"{"data":{"object":{"metadata":{"presentation":{"label":"Local demo"}}}},"pad":""}"#;
        let body = base.replace(
            "\"pad\":\"\"",
            &format!("\"pad\":\"{}\"", "x".repeat(size - base.len() - 16)),
        ) + "                ";
        assert_eq!(body.len(), size);
        for concurrency in [1, 4] {
            for mode in ["copy", "native_identity", "wasm_identity", "wasm_move"] {
                let barrier = Arc::new(Barrier::new(concurrency));
                let mut threads = Vec::new();
                let started = Instant::now();
                for _ in 0..concurrency {
                    let runtime = runtime.clone();
                    let barrier = barrier.clone();
                    let body = body.clone();
                    threads.push(std::thread::spawn(move || {
                        let plan = if mode == "wasm_move" {
                            Plan::MoveString {
                                from: "/data/object/metadata/presentation/label".into(),
                                path: "/data/object/metadata/display_label".into(),
                            }
                        } else {
                            Plan::Identity
                        };
                        let input = Input {
                            abi_version: 1,
                            plan,
                            body,
                        };
                        let mut timings = Vec::new();
                        let mut failures = 0;
                        let mut first_error = None;
                        barrier.wait();
                        for _ in 0..50 {
                            let start = Instant::now();
                            let error = match mode {
                                "copy" => {
                                    std::hint::black_box(input.body.clone());
                                    None
                                }
                                "native_identity" => {
                                    adapter_core::transform(&input).err().map(str::to_string)
                                }
                                _ => match runtime.run(&input) {
                                    Ok(adapter_core::Output::Rejected { code }) => {
                                        Some(format!("guest_rejected:{code}"))
                                    }
                                    Ok(_) => None,
                                    Err(error) => Some(error.to_string()),
                                },
                            };
                            timings.push(start.elapsed().as_secs_f64() * 1_000_000.0);
                            if error.is_some() {
                                failures += 1;
                                if first_error.is_none() {
                                    first_error = error;
                                }
                            }
                        }
                        (timings, failures, first_error)
                    }));
                }
                let mut timings = Vec::new();
                let mut failures = 0;
                let mut first_error = None;
                for thread in threads {
                    let (times, failed, error) = thread.join().unwrap();
                    timings.extend(times);
                    failures += failed;
                    if first_error.is_none() {
                        first_error = error;
                    }
                }
                timings.sort_by(f64::total_cmp);
                let percentile = |p: f64| timings[((timings.len() - 1) as f64 * p).ceil() as usize];
                results.push(json!({"body_bytes":size,"concurrency":concurrency,"mode":mode,"attempts":timings.len(),"failures":failures,"first_error":first_error,"p50_us":percentile(0.5),"p95_us":percentile(0.95),"p99_us":percentile(0.99),"elapsed_ms":started.elapsed().as_secs_f64()*1000.0}));
            }
        }
    }
    let read = |path| std::fs::read_to_string(path).ok();
    println!(
        "{}",
        serde_json::to_string_pretty(
            &json!({"kind":"runtime_microbenchmark_not_gateway_qualification","os":std::env::consts::OS,"architecture":std::env::consts::ARCH,"logical_cpus":std::thread::available_parallelism()?.get(),"kernel":read("/proc/sys/kernel/osrelease"),"memory_info":read("/proc/meminfo").map(|s|s.lines().take(3).collect::<Vec<_>>().join("\n")),"process_status":read("/proc/self/status").map(|s|s.lines().filter(|s|s.starts_with("VmHWM:")||s.starts_with("VmRSS:")).collect::<Vec<_>>().join("\n")),"process_cpu_stat":read("/proc/self/stat"),"cpu_max":read("/sys/fs/cgroup/cpu.max"),"memory_max":read("/sys/fs/cgroup/memory.max"),"wasmtime":"49.0.3","rust":"1.99.1","artifact_sha256":format!("{:x}",Sha256::digest(&wasm)),"compile_ms":compile_ms,"instance_policy":"fresh instance per invocation; compiled module reused","fuel":executor::FUEL,"deadline_ms":10,"results":results})
        )?
    );
    Ok(())
}
