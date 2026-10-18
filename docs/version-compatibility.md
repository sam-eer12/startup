# Week 1 compatibility record

Pinned on 18 October 2026. Registry metadata confirms Linux AMD64 and ARM64
manifests for every image below; that is availability evidence, not runtime
qualification. Full image digests are committed in
[images.env](../deploy/compose/images.env).

| Component | Pin | Qualification |
|---|---|---|
| Go | 1.27.2; `golang:1.27.2-bookworm` | Native macOS ARM64 build, race tests and vet; Linux image available |
| Rust | 1.99.1; `rust:1.99.1-bookworm` | Native macOS ARM64 workspace/oracle; Linux image available |
| Python | 3.14.8 | Schema/provenance tooling; exact package versions and hashes in `requirements-dev.txt` |
| Envoy | `v1.36.13` | Image digest resolved; local HTTP bootstrap only; ext_proc qualification pending Week 2 |
| PostgreSQL | `18.6-bookworm` | Image digest resolved; native application integration pending |
| NATS | `2.12.4-alpine` | Image digest resolved; JetStream config provided; outbox/inbox integration pending |
| Docker / Compose | Engine/Desktop plus Compose with `up --wait`; temporary CLI 5.6.0 used for validation | Compose configuration validated; Docker engine unavailable; full Linux startup pending CI/developer Docker run |
| Wasmtime / Tonic / Prost | Not yet selected | Week 2 spike must pin and qualify before runtime claims |
| Wasm ABI | Version 1 reserved in manifests | Memory ownership, host exports/imports and executable ABI pending Week 2 |
| Consumer schema dialect | JSON Schema 2020-12 | Synthetic contract validation only; bounded import subset pending Week 3 |
| OpenAPI import | 3.1.1 planned | Not implemented |
| All four fixture packs | `fixture-v1` | Synthetic fixture formats only; no provider API release or live-account compatibility claimed |

Go has no external modules yet. Rust dependencies are locked in `Cargo.lock` with
serde_json's arbitrary-precision number feature. Python requirements are
hash-locked. Scanning uses govulncheck 1.9.0 and cargo-audit 0.22.2 with current
advisory databases; scanners are development tools, not request-path dependencies.
CI action commits are pinned in the workflow. Runtime dependency compatibility,
supported provider versions and image security eligibility must be reviewed again
before any pilot deployment.

Sources used for pins and interfaces:
[Go releases](https://go.dev/dl/),
[Rust stable channel metadata](https://static.rust-lang.org/dist/channel-rust-stable.toml),
[Envoy image documentation](https://www.envoyproxy.io/docs/envoy/v1.36.0/start/docker),
[OCI registry API](https://distribution.github.io/distribution/spec/api/),
[JSON Schema 2020-12](https://json-schema.org/draft/2020-12).
