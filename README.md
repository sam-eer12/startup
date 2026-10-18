# Self-Healing API Mesh

Week 1 foundation and Week 2 technical spikes for the [project plan](docs/implementation-plan.md):
a Go simulator/collector, Rust/Wasm transforms behind Envoy ext_proc, independent
oracle, signature fixtures, and an encrypted object-first/PostgreSQL acceptance lab.

The collector still collects **redacted, non-authoritative observations**. Signature
and durable-acceptance code is exercised by synthetic local tests, with no provider-facing
acceptance endpoint. Drift detection, compilation and deployment remain later milestones.
See [Week 2 status](docs/week-2.md) for evidence, limits and the open M0 gate.

## Run locally

Install Go **1.27.2**, Rust **1.99.1** with rustfmt/clippy, and Python **3.14.8**.
Rust's toolchain file selects the pinned version when using rustup.

```sh
rustup target add wasm32-unknown-unknown --toolchain 1.99.1
```

```sh
make bootstrap
make check
make dev
```

In another terminal:

```sh
make smoke
go run ./cmd/meshctl health
curl http://127.0.0.1:8081/fixtures/stripe/field-move
curl -H 'Content-Type: application/json' \
  --data-binary @fixtures/synthetic/stripe/field-move.json \
  http://127.0.0.1:8081/observe/route_stripe
```

The response contains a `redacted_object_ref`. Its SHA-256 suffix identifies the
sample in `.local/objects/<digest>.json`. Samples contain masked scalar values and
property names; raw bodies and headers are never persisted. Objects expire after
24 hours after their latest capture, swept on writes/startup and every minute while the process runs. Storage
is capped at 64 MiB / 10,000 objects per single-writer directory. Stopping the local
process also stops its retention sweeper; the next startup removes expired samples.
Use only synthetic data in this development profile.

## Run the Linux stack

Podman and Docker Compose with `up --wait` support are supported; Make defaults to
`podman compose`. Start the existing VM with `podman machine start` on macOS.
For Docker Engine/Desktop, pass `COMPOSE_PROVIDER="docker compose"` to Make.
Image references are locked by
multi-platform index digest in [images.env](deploy/compose/images.env).

```sh
make bootstrap
make compose-check
make up
make smoke SMOKE_URL=http://127.0.0.1:8080
make down
```

Compose starts Envoy, separate simulator/collector containers, PostgreSQL, and NATS
JetStream. Only the gateway (`127.0.0.1:8080`) and Envoy admin
(`127.0.0.1:9901`) are exposed to the host. PostgreSQL and NATS are infrastructure
foundations: the Week 1 collector writes redacted filesystem samples and does not
yet use them for a ledger or outbox. `make down` retains the local volumes.

To run the independent oracle in a network-disabled Linux container:

```sh
podman compose --env-file deploy/compose/images.env \
  -f deploy/compose/compose.yaml run --rm --build oracle
```

## Week 2 synthetic lab

```sh
make week2-up
make week2-check
go run ./cmd/signature-demo
```

The checks run inside the Linux network, including real PostgreSQL process-crash
tests and Envoy response transformation/framing checks. They do not depend on
macOS host port forwarding. `/runtime/identity` and `/runtime/move` are synthetic
fixture routes; `/observe/...` retains its original unverified capture behavior.
The signature demo writes disposable fixtures/public keys to `.local/week2-signatures`.
Private signing keys are never written. These fixtures expire under the timestamp policy.

To run the diagnostic gateway sweep and the network-disabled runtime microbenchmark:

```sh
podman compose --env-file deploy/compose/images.env -f deploy/compose/compose.yaml \
  -f deploy/compose/week2.yaml run --rm --no-deps durability-tests \
  python3 scripts/week2_bench.py --url http://envoy:8080
podman run --rm --network none --entrypoint /src/target/release/runtime-bench \
  mesh-local-executor /src/target/wasm32-unknown-unknown/release/adapter_wasm.wasm
make week2-down
```

The [ABI/body/trust decision](docs/decisions/0002-runtime-trust-spike.md) defines the
limits. Measurements on the current smaller Podman VM do not satisfy the planned
4-vCPU/8-GiB qualification. No customer capture or production webhook registration
is authorized by this lab.

## Development

- `make build`: build the Go binaries and Rust workspace.
- `make check`: formatting, static checks, schemas/provenance, Go race tests, Rust tests, oracle.
- `make fmt`: apply Go, Rust, and Python formatting.
- `make scan`: Go vulnerability check and `cargo audit` (install `cargo-audit 0.22.2`; network required).
- `python3 scripts/seed_fixtures.py --check`: detect changes to the reviewed synthetic corpus.
- `python3 scripts/lock_images.py`: explicitly refresh image digests; review and revalidate afterward.

CI is configured to run these checks and the complete Compose smoke path on Linux. Python tools
are pinned with hashes; Rust dependencies use `Cargo.lock`, and Go dependencies use
`go.mod`/`go.sum`. CI actions are pinned by commit.

The [schemas](schemas/README.md), [route decisions](docs/decisions/0001-local-foundation.md),
[sized work plan](docs/week-1-capacity.md), and [original architecture](docs/architecture.md)
describe the boundaries for the next milestones.
