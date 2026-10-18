pub mod ext_proc;

use adapter_core::{Input, MAX_BODY, MAX_ENVELOPE, Output};
use anyhow::{Context, Result, bail, ensure};
use std::{
    sync::{Arc, Weak},
    time::{Duration, Instant},
};
use wasmtime::{Config, Engine, Instance, Module, Store, StoreLimits, StoreLimitsBuilder};

pub const FUEL: u64 = 50_000_000;
pub const MEMORY: usize = 32 * 1024 * 1024;
pub const DEADLINE: Duration = Duration::from_millis(10);

pub struct Runtime {
    engine: Arc<Engine>,
    module: Module,
}

impl Runtime {
    pub fn new(wasm: &[u8]) -> Result<Self> {
        ensure!(wasm.len() <= 4 * 1024 * 1024, "artifact too large");
        let mut config = Config::new();
        config.consume_fuel(true).epoch_interruption(true);
        let engine = Arc::new(Engine::new(&config)?);
        let module = Module::new(&engine, wasm)
            .map_err(anyhow::Error::from)
            .context("compile core Wasm")?;
        ensure!(
            module.imports().next().is_none(),
            "guest imports are forbidden"
        );
        let weak: Weak<Engine> = Arc::downgrade(&engine);
        std::thread::spawn(move || {
            loop {
                std::thread::sleep(Duration::from_millis(1));
                let Some(engine) = weak.upgrade() else {
                    break;
                };
                engine.increment_epoch();
            }
        });
        Ok(Self { engine, module })
    }

    pub fn run(&self, input: &Input) -> Result<Output> {
        ensure!(input.body.len() <= MAX_BODY, "input body limit");
        if let adapter_core::Plan::MoveString { from, path } = &input.plan {
            ensure!(
                from.len() <= 1024 && path.len() <= 1024,
                "plan pointer limit"
            );
        }
        let bytes = serde_json::to_vec(input)?;
        ensure!(
            bytes.len() <= MAX_ENVELOPE && input.body.len() <= MAX_BODY,
            "input limit"
        );
        let started = Instant::now();
        let limits = StoreLimitsBuilder::new()
            .memory_size(MEMORY)
            .memories(1)
            .instances(1)
            .tables(1)
            .table_elements(10_000)
            .trap_on_grow_failure(true)
            .build();
        let mut store: Store<StoreLimits> = Store::new(&self.engine, limits);
        store.limiter(|limits| limits);
        store.set_fuel(FUEL)?;
        store.set_epoch_deadline(10);
        let instance = Instance::new(&mut store, &self.module, &[])?;
        let version = instance.get_typed_func::<(), u32>(&mut store, "abi_version")?;
        ensure!(version.call(&mut store, ())? == 1, "unsupported ABI");
        let memory = instance
            .get_memory(&mut store, "memory")
            .context("missing memory")?;
        let alloc = instance.get_typed_func::<u32, u32>(&mut store, "alloc")?;
        let dealloc = instance.get_typed_func::<(u32, u32), ()>(&mut store, "dealloc")?;
        let transform =
            instance.get_typed_func::<(u32, u32, u32, u32), i32>(&mut store, "transform")?;
        let ptr = alloc.call(&mut store, bytes.len() as u32)? as usize;
        let out = alloc.call(&mut store, MAX_ENVELOPE as u32)? as usize;
        let end = ptr
            .checked_add(bytes.len())
            .context("input range overflow")?;
        let out_end = out
            .checked_add(MAX_ENVELOPE)
            .context("output range overflow")?;
        ensure!(
            ptr != 0
                && out != 0
                && end <= memory.data_size(&store)
                && out_end <= memory.data_size(&store),
            "invalid allocation range"
        );
        ensure!(end <= out || out_end <= ptr, "overlapping buffers");
        memory.write(&mut store, ptr, &bytes)?;
        let len = transform.call(
            &mut store,
            (
                ptr as u32,
                bytes.len() as u32,
                out as u32,
                MAX_ENVELOPE as u32,
            ),
        )?;
        ensure!(
            len > 0 && len as usize <= MAX_ENVELOPE,
            "invalid output length"
        );
        let mut result = vec![0; len as usize];
        memory.read(&store, out, &mut result)?;
        dealloc.call(&mut store, (ptr as u32, bytes.len() as u32))?;
        dealloc.call(&mut store, (out as u32, MAX_ENVELOPE as u32))?;
        ensure!(started.elapsed() <= DEADLINE, "execution deadline");
        let output: Output = serde_json::from_slice(&result).context("invalid output envelope")?;
        if let Some(body) = output.body() {
            adapter_core::validate_json(body.as_bytes()).map_err(anyhow::Error::msg)?;
        }
        match &output {
            Output::Unchanged { body } if body != &input.body => {
                bail!("unchanged result changed bytes")
            }
            Output::Rejected { code } if code.len() > 64 => bail!("invalid rejection code"),
            _ => {}
        }
        Ok(output)
    }
}
