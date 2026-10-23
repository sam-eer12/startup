# Local compatibility record

Initial pins recorded on 18 October 2026; Go/Wasmtime security updates on 23 October 2026.
Registry metadata confirms Linux AMD64 and ARM64
manifests for every image below; that is availability evidence, not runtime
qualification. Full image digests are committed in
[images.env](../deploy/compose/images.env).

| Component | Pin | Qualification |
|---|---|---|
| Go | 1.27.3; `golang:1.27.3-bookworm` | Required security floor for GO-2026-6809; native and CI checks described below |
| Rust | 1.99.1; `rust:1.99.1-bookworm` | Native macOS ARM64 workspace/oracle; Linux image available |
| Python | 3.14.8 | Schema/provenance tooling; exact package versions and hashes in `requirements-dev.txt` |
| Envoy | `v1.36.13` | Podman Linux synthetic response transforms, chunking/framing and rejection smoke; production qualification pending |
| PostgreSQL | `18.6-bookworm` | Podman Linux object-first/event+job commit and real process-crash tests; no replicated durability claim |
| NATS | `2.12.4-alpine` | Podman health check passed; outbox publisher/inbox integration pending |
| Podman / Compose | Podman 6.0.2 + Docker Compose 5.4.0 | Linux ARM64 local stack; current VM 2 vCPU/~3 GiB; host port forwarding resets, in-network tests pass |
| Wasmtime / Tonic / Prost | 49.0.4 / 0.14.6 / 0.14.4; envoy-types 0.7.7 | Wasmtime security patch; exact dependency graph in Cargo.lock; original Linux spike measurements used 49.0.3 |
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

Go 1.27.3 fixes the standard-library `crypto/x509` advisory
[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809), which failed the 23 October
`make scan` run with Go 1.27.2. The module's minimum Go version, CI setup and
Linux image pin move together; the scan remains enforced. `go mod tidy` removes
the redundant toolchain directive once the minimum itself is 1.27.3.
Historical Week 2 scan evidence predates this advisory and is not current clearance.

The subsequent Rust scan identified
[RUSTSEC-2026-0378](https://rustsec.org/advisories/RUSTSEC-2026-0378.html) and
[RUSTSEC-2026-0379](https://rustsec.org/advisories/RUSTSEC-2026-0379.html) in
Wasmtime 49.0.3. The executor and its locked Wasmtime/Cranelift family move to the
49.0.4 security release. Sandbox features, fuel, body and deadline limits remain
unchanged; historical runtime benchmarks do not qualify the patched build.

Patch validation on 23 October: `make check build`, `make week3-smoke` and
`make scan` pass on macOS ARM64 with Go 1.27.3, Rust 1.99.1 and 1,351 RustSec
advisories. Both scanners remain mandatory in CI. The Go image index was resolved
from Docker Hub and checked for Linux AMD64/ARM64 manifests; updated container
runtime validation runs separately in GitHub Actions.

Sources used for pins and interfaces:
[Go releases](https://go.dev/dl/),
[Rust stable channel metadata](https://static.rust-lang.org/dist/channel-rust-stable.toml),
[Envoy image documentation](https://www.envoyproxy.io/docs/envoy/v1.36.0/start/docker),
[OCI registry API](https://distribution.github.io/distribution/spec/api/),
[JSON Schema 2020-12](https://json-schema.org/draft/2020-12).
