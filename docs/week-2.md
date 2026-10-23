# Week 2 feasibility and execution status

18 October 2026. Local technical spikes are implemented. **M0 remains open**:
the specified 4-vCPU/8-GiB performance qualification, partner inputs and confirmed
owner estimates are not available. This is not a production webhook service.

| Work package | Evidence | Outcome |
|---|---|---|
| Identity and guarded move in shared Rust core | `crates/adapter-core`, `adapter-wasm`, executor integration tests | Native/Wasm equivalence across four synthetic families; independent Stripe golden assertions; exact no-op bytes and numbers |
| Core ABI and runtime limits | [ADR 0002](decisions/0002-runtime-trust-spike.md), `crates/executor` | Executable ABI v1; no imports; memory/fuel/deadline/range limits; fresh instances |
| Envoy ext_proc | `deploy/envoy/week2.yaml`, real HTTP/2 Tonic test, `scripts/week2_smoke.py` | Linux no-op/move, chunking, empty bodies, malformed-body rejection, header cleanup and concurrent requests |
| Original and mesh signatures | `internal/trust`, `cmd/signature-demo` | Stripe SDK authenticates original bytes; mutation fails; detached mesh JWS verifies exact envelope bytes with scope/audience/time/data checks; Shopify HMAC fixture |
| Object-first durable acceptance | `internal/durable` and real PostgreSQL tests | AES-GCM object fsync before event+pending-job commit; process-exit matrix and retry/conflict behavior pass |
| Data/export boundary | Existing Go capture/integration tests plus encrypted-object tests | Seeded secret keys/values/headers/query stay out of telemetry; interrupted/compressed/unsupported bodies rejected; operational bytes encrypted separately |
| Runtime/gateway performance | `scripts/week2_bench.py`, executor `runtime-bench`, JSON evidence below | Diagnostic results on current smaller Podman VM; no capacity/SLO qualification |
| Proxy-Wasm comparison | Plan descope item 2 | Deferred; ext_proc remains primary |
| NetSuite/live provider access | Synthetic fixture pack retained | No authorized sandbox/account or exact live read operation supplied; qualification pending |

## Verification

`make check build` passes formatting, static checks, schemas/provenance, Go race
tests, Rust unit/runtime/HTTP2 tests, golden oracle and builds. The native Go run
skips the opt-in PostgreSQL test when no database URL is supplied; the Linux test
runner executes that test against the real PostgreSQL container.

The full Go race suite and abrupt-crash matrix pass on Linux ARM64 under Podman.
The independent Rust oracle also passes in a network-disabled Linux container.
Envoy and the executor start under Compose; the Envoy image's log-device `chown`
entrypoint was bypassed with its direct binary and an unprivileged UID for Podman.
`govulncheck 1.9.0` reports no vulnerabilities after updating x/text to 0.41.0.
`cargo-audit 0.22.2` reports none against 1,338 RustSec advisories; its optional
registry-index refresh warned, so no new yanked-package eligibility claim is made.
Remote CI is configured for these paths but has not been run in this session.

## Crash-window matrix

Tests exit a child process with code 86 at each boundary; deferred rollback/cleanup
does not run. PostgreSQL recovery and a fresh object-store instance are then used.

| Crash point | Event/job committed before retry | Storage state | Retry |
|---|---|---|---|
| Before object write | No | No new object | Creates one event and pending job |
| After object, before DB commit | No | Authenticated encrypted orphan possible | Reuses object, commits event/job |
| After DB commit | Yes | Required object readable after reopen | Returns original logical event; one pending job |
| Before response reaches caller | Yes | Required object readable after reopen | Same dedupe behavior; no duplicate job |

Every case also checks repeated retry and changed-bytes/same-ID rejection. This
does not prove replicated commit, machine power-loss recovery or downstream
business effects. There is no provider-facing durable-acceptance endpoint, job
publisher/worker, automatic orphan GC, receiver inbox or production KMS integration.

## Diagnostic measurements

The VM is Linux ARM64, 2 vCPU and approximately 3 GiB, with Podman 6.0.2 and Docker
Compose 5.4.0. Fixed synthetic payloads contain one large padding string, not a
representative provider distribution. Both input and output must fit 256 KiB;
an otherwise valid move that expands beyond the output budget is rejected. The
boundary regression test covers this explicitly. Benchmark rejections count as
failures, rather than successful Wasmtime calls.

The runtime sweep reuses a compiled module with a fresh instance per call, 50 calls
per worker at concurrency 1/4, and includes copy/native-no-op baselines. Sixteen
trailing whitespace bytes leave room for this fixture's longer destination key.
Cold module compilation, memory high-water and process CPU counters are recorded.
The gateway sweep uses formatted fixture JSON plus synthetic padding, 10 warmups
and 100 requests per cell, new HTTP/1.1 client connections and HTTP/2 ext_proc.
It includes successful bypass/no-op/move requests and validates protected fixture
fields on every success. It is a short sweep, not a sustained 500 requests/s test.
The first transform request immediately after executor replacement returned 504;
the path recovered once the gateway connection settled. Cold connection/readiness
behavior remains unqualified under the unchanged 25 ms processing deadline.

Raw evidence: [runtime](evidence/week2-runtime-linux.json), [gateway](evidence/week2-gateway-linux.json), [service resources](evidence/week2-resources.json).

| Input | Callers | Gateway path | p50 ms | p95 ms | p99 ms | Failures / attempts |
|---|---:|---|---:|---:|---:|---:|
| 32 KiB | 1 | bypass | 0.508 | 1.216 | 2.335 | 0 / 100 |
| 32 KiB | 1 | identity | 1.339 | 1.674 | 2.904 | 0 / 100 |
| 32 KiB | 1 | move | 1.344 | 1.537 | 2.017 | 0 / 100 |
| 32 KiB | 4 | bypass | 1.347 | 2.788 | 4.195 | 0 / 100 |
| 32 KiB | 4 | identity | 3.027 | 4.625 | 5.294 | 0 / 100 |
| 32 KiB | 4 | move | 3.226 | 5.023 | 5.960 | 0 / 100 |
| 256 KiB | 1 | bypass | 1.114 | 2.366 | 3.599 | 0 / 100 |
| 256 KiB | 1 | identity | 2.814 | 4.141 | 7.570 | 0 / 100 |
| 256 KiB | 1 | move | 3.176 | 4.286 | 5.248 | 0 / 100 |
| 256 KiB | 4 | bypass | 3.343 | 6.363 | 9.006 | 0 / 100 |
| 256 KiB | 4 | identity | 7.110 | 10.389 | 12.656 | 0 / 100 |
| 256 KiB | 4 | move | 8.524 | 14.114 | 16.019 | 0 / 100 |

The runtime sweep recorded 0 failures across 2000 invocations. Cold module compilation took 148.4 ms. Runtime-process memory: `VmHWM:	   38608 kB; VmRSS:	   24660 kB`. CPU time including compilation: 0.95 s.

For 32 KiB/four callers, gateway p95 was 2.788 ms bypass versus 5.023 ms move. This is a comparison of separate samples, not a paired added-latency percentile or sustained capacity proof. The sweep had zero warmup failures after the separately observed cold replacement timeout.

The macOS Podman host loopback forwarding reset connections during this session;
all Linux service checks ran on the isolated Compose network. This leaves host
port forwarding unqualified. It does not prevent the reproducible container checks.

## Reproduce

```sh
podman machine start                 # only if the existing VM is stopped
make bootstrap
make check build
make week2-up week2-check
go run ./cmd/signature-demo
make week2-down                     # keeps local volumes
```

Use `COMPOSE_PROVIDER="docker compose"` with Make on Docker-equipped machines.
The [README](../README.md#week-2-synthetic-lab) includes the two benchmark commands.
Generated signature fixtures/public keys live in `.local/week2-signatures` and
expire under the five-minute timestamp policy; disposable private keys are not saved.
Fixture adapter/contract digests are labeled identity markers, not signed release evidence.

## Open M0 decisions and scope

- Repeat representative payload and sustained-load measurements on the declared
  4-vCPU/8-GiB Linux profile, including cold gateway startup, saturation, failure
  rates, CPU/memory under load and the synchronous 25 ms path. Re-baseline the
  body/concurrency envelope if deadlines fail; do not weaken limits to hide misses.
- Confirm selected partner routes, named invariant owners, exact provider releases,
  locality/retention, consumer mesh trust and deployment boundary. Current contracts
  remain synthetic fixtures, and the W0 partner/pilot clock has not started.
- Obtain Stripe/Shopify test accounts and authorized NetSuite sandbox/read operation
  before claiming live connector qualification. NetSuite remains fixture-only.
- Confirm engineering ownership and sized estimates against the 12 engineer-day
  weekly cap. Proxy-Wasm is deferred; no owner-confirmed schedule re-baseline is claimed.
- For future eBPF work, obtain each partner's Linux architecture/kernel/BTF,
  permitted capabilities, TLS library/build, Go runtime/linking/stripping details,
  HTTP/2/compression workload and restart behavior. No partner capture agent is built.

The next independent product milestone is Week 3 contract IR/registry and labeled
drift detection; an [initial synthetic slice](week-3.md) is now implemented.
The [M0 tracker](m0-closure.md) records the remaining closure evidence. These spikes do not implement synthesis, general schema import,
approvals, release eligibility, canary/rollback or production delivery.
