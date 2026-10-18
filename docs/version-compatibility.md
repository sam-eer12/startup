# Local compatibility record

Pinned on 18 October 2026. Registry metadata confirms Linux AMD64 and ARM64
manifests for every image below; that is availability evidence, not runtime
qualification. Full image digests are committed in
[images.env](../deploy/compose/images.env).

| Component | Pin | Qualification |
|---|---|---|
| Go | 1.27.2; `golang:1.27.2-bookworm` | Native macOS ARM64 build, race tests and vet; Linux image available |
| Rust | 1.99.1; `rust:1.99.1-bookworm` | Native macOS ARM64 workspace/oracle; Linux image available |
| Python | 3.14.8 | Schema/provenance tooling; exact package versions and hashes in `requirements-dev.txt` |
| Envoy | `v1.36.13` | Podman Linux synthetic response transforms, chunking/framing and rejection smoke; production qualification pending |
| PostgreSQL | `18.6-bookworm` | Podman Linux object-first/event+job commit and real process-crash tests; no replicated durability claim |
| NATS | `2.12.4-alpine` | Podman health check passed; outbox publisher/inbox integration pending |
| Podman / Compose | Podman 6.0.2 + Docker Compose 5.4.0 | Linux ARM64 local stack; current VM 2 vCPU/~3 GiB; host port forwarding resets, in-network tests pass |
| Wasmtime / Tonic / Prost | 49.0.3 / 0.14.6 / 0.14.4; envoy-types 0.7.7 | Native/Wasm equivalence and Linux ext_proc spike; exact dependency graph in Cargo.lock |
| Wasm ABI | Executable v1 | No imports; caller-owned buffers; fresh bounded instances; [ADR 0002](decisions/0002-runtime-trust-spike.md) |
| Consumer schema dialect | JSON Schema 2020-12 | Synthetic contract validation only; bounded import subset pending Week 3 |
| OpenAPI import | 3.1.1 planned | Not implemented |
| All four fixture packs | `fixture-v1` | Synthetic fixture formats only; no provider API release or live-account compatibility claimed |

Go dependencies include stripe-go/v84 84.4.1, go-jose/v4 4.1.6, pgx/v5 5.11.0 and
x/text 0.41.0, pinned with checksums. Rust dependencies are locked in `Cargo.lock` with
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
