# Self-Healing API Mesh

Week 1 foundation for the [project plan](docs/implementation-plan.md): a Go local
simulator/collector, Rust contract/DSL vocabulary and independent oracle, versioned
JSON schemas, synthetic connector fixtures, and a Linux Compose environment.

The current service collects **redacted, non-authoritative observations**. Provider
signature verification, drift detection, compilation, durable webhook acceptance,
and deployment are later milestones. See [Week 1 status](docs/week-1.md) for tested
behavior, remaining checks, and partner dependencies.

## Run locally

Install Go **1.27.2**, Rust **1.99.1** with rustfmt/clippy, and Python **3.14.8**.
Rust's toolchain file selects the pinned version when using rustup.

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

Docker Engine/Desktop and Compose with `up --wait` support are required. Image references are locked by
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
docker compose --env-file deploy/compose/images.env \
  -f deploy/compose/compose.yaml run --rm --build oracle
```

## Development

- `make build`: build the Go binaries and Rust workspace.
- `make check`: formatting, static checks, schemas/provenance, Go race tests, Rust tests, oracle.
- `make fmt`: apply Go, Rust, and Python formatting.
- `make scan`: Go vulnerability check and `cargo audit` (install `cargo-audit 0.22.2`; network required).
- `python3 scripts/seed_fixtures.py --check`: detect changes to the reviewed synthetic corpus.
- `python3 scripts/lock_images.py`: explicitly refresh image digests; review and revalidate afterward.

CI is configured to run these checks and the complete Compose smoke path on Linux. Python tools
are pinned with hashes; Rust dependencies use `Cargo.lock`. The Go service currently
has no external modules, so it needs no `go.sum`. CI actions are pinned by commit.

The [schemas](schemas/README.md), [route decisions](docs/decisions/0001-local-foundation.md),
[sized work plan](docs/week-1-capacity.md), and [original architecture](docs/architecture.md)
describe the boundaries for the next milestones.
