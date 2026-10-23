# Week 3: local contract registry and drift diagnostics

23 October 2026. An initial **synthetic-only** Week 3 slice is implemented.
[M0 remains open](m0-closure.md); this does not start the partner/pilot clock.

| Plan item | Implemented evidence | Remaining boundary |
|---|---|---|
| Bounded import and typed IR | Rust `contract-ir` library/offline executable; [keyword matrix](contract-subset.md); exact decimal comparison, local reference limits | JSON serialization only; selected OpenAPI schema pointer, not whole-operation import or YAML |
| Versions, hashes, provenance and bindings | Go `internal/control`; explicit immutable imports; source and IR SHA-256; explicit published local route bindings | Private single-writer filesystem lab; authoritative PostgreSQL registry remains to be built |
| Observed shape | Required absence/null, exact integer/decimal types, empty/heterogeneous arrays, matching union branch indexes; only contract-approved names returned | Per-observation projection; longitudinal histograms/inference, confidence calibration and request denominators remain open |
| Classification and cohorts | Structural, operational, semantic-feedback and unsupported classes; fixed route metadata and bound contract version in fingerprints | Semantic detection requires explicit local fixture feedback; sequence diagnostics and provider-qualified invariants remain open |
| Incident lifecycle | Provisional immediately; recurring structural mismatch confirms after three complete observations within five minutes; explicit invariant feedback confirms immediately; owner, first/last seen, evidence coverage | Confirmation is diagnostic, not repair eligibility. No payload/replay evidence is retained |
| Operator workflow | Loopback API plus `meshctl` import, bind, drift, list/detail and mapping-review commands | No production authentication, approvals, synthesis, deployment or release path |
| Labeled evaluation | **73/73** hand-authored scenarios; separate JSON Schema validator checks structural labels; [raw report](evidence/week3-detector-local.json) | Synthetic evaluation, no partner detection-precision or release qualification claim |

## Run and review

```sh
make build
make week3-check
make week3-dev
```

The opt-in control lab binds `127.0.0.1:8082`, independently of the existing
simulator/collector and Compose gateway. Only the four embedded synthetic route
IDs and `tenant_local` / `env_local` contracts with `synthetic_fixture` authority
are accepted. `owner_approved` input is rejected. No existing `/observe` traffic
is routed into the registry; fully redacted captures cannot prove field drift.

Prepare three request files from repository-owned fixtures:

```sh
.venv/bin/python scripts/week3_requests.py
bin/meshctl -url http://127.0.0.1:8082 import .local/week3-requests/import.json
bin/meshctl -url http://127.0.0.1:8082 bind .local/week3-requests/bind.json
bin/meshctl -url http://127.0.0.1:8082 drift .local/week3-requests/drift.json
bin/meshctl -url http://127.0.0.1:8082 incidents
bin/meshctl -url http://127.0.0.1:8082 incident inc_HASH_FROM_RESPONSE
```

Repeat `drift` three times within five minutes to inspect confirmation. These are
separate synthetic diagnostic submissions, not three distinct production events.
Mapping review takes a JSON file containing `from`, `to` JSON Pointers and a named
`reviewer`: `meshctl -url http://127.0.0.1:8082 review inc_HASH review.json`.
It records `authorizes_repair: false`; there is no approval side effect.

The API accepts the same request files at `POST /v1/contracts`, `/v1/bindings`,
`/v1/drift` and `/v1/incidents/{id}/mapping-review`. `GET /v1/contracts`,
`/v1/bindings`, `/v1/incidents` and `/v1/incidents/{id}` expose local state.
Lists use `limit=1..100` (default 50) and an `after` key returned as `next`;
incidents also accept `route_id`. CLI lists display the first page and its cursor.
All POST requests require `Content-Type: application/json`. Browser-origin
requests are blocked. This is a local lab, not authenticated tenant isolation.

An import file contains `contract` (the existing fixture envelope), `pointer`
(empty for a JSON Schema root), and a `source_label` identifier. For OpenAPI JSON,
put the full document in `contract.schema`, set the local import dialect to
`openapi-3.0` or `openapi-3.1`, and supply an explicit schema JSON Pointer. These
local import envelopes extend the v1 wire contract dialect; they do not change
the published v1 schema or claim full OpenAPI import support.

## Storage, coverage and limits

The private `.local/week3-registry/registry.json` snapshot uses fsync plus atomic
replacement, a process-lifetime writer lock, a maximum of 256 contract versions,
1,024 incidents, 32 recent evidence metadata entries per incident and 8 MiB total.
There is no implicit eviction or production retention service. It stores explicit
synthetic contract sources, provenance, bindings, incident metadata and mapping
annotations until the operator removes the disposable lab directory.

Drift bodies are ephemeral. Reports omit all scalar values and unknown property
names; known contract paths, exact value *types*, missing required fields and
unknown-member counts remain available. Evidence references identify metadata
entries within an incident, not retained replay payloads. No raw body, request
headers or URL query is persisted. Source imports are explicit trusted fixture
inputs and may contain declared constants; never import secrets as contracts.

`sample_probability` is stored separately from contract truth and per-observation
confidence. No estimated affected-traffic count is invented without denominators.
A `healthy` result means only that this complete JSON observation satisfies the
declared subset. Unit or business errors can satisfy the schema: absent explicit
invariants, `semantic_verification` remains unavailable. There is no automatic
rename inference, mapping guess, schema publication from samples, or automatic patch.

The Rust tool accepts one ≤256-KiB combined operation envelope per process. Bodies
and schemas share that budget after JSON string escaping. JSON depth is bounded
to 48 and 10,000 nodes; expanded schemas to depth 32 / 4,096 nodes / 64 references;
unions to eight branches, reported changes to 64 and validation/shape work to
50,000 visits. The Go service permits one registry operation at a time, bounds
engine output to 2 MiB and kills an engine operation after three seconds. These
are diagnostic limits, separate from the synchronous 25 ms runtime deadline.

## Validation and next work

`make check build` includes the Go race suite, Rust parser/runtime suites,
schema/provenance checks, independent transform oracle and the 73-case detector
evaluation. `make week3-check` focuses on the new slice. Tests exercise restart,
immutable versions, cohort separation, confirmation windows, rejected evidence,
redaction, persistence failure and exclusive-writer behavior.

The Week 2 Linux Go-test image now builds/copies the offline Rust engine so its
existing full Go suite can exercise these integration tests too. The control lab
itself has no Compose/public gateway deployment in this increment.

`make check build`, `make week3-check` and the real CLI/HTTP/restart smoke
(`make week3-smoke`) passed locally on macOS ARM64. The updated Linux container
path was not executed: Podman reported startup, then its VM returned to `stopped`
and the socket refused connections. Remote CI is configured but has not been run
in this session. This does not supply reference-hardware performance evidence.

Next Week 3 work is the PostgreSQL registry implementation and migration,
longitudinal observed-shape/coverage summaries, and partner-reviewed import and
invariant requirements once [external inputs](m0-closure.md) arrive. Broader schema
keywords require explicit semantics and conformance fixtures before enabling them.
Week 3 has started; its full exit and M0 sign-off are not asserted.
