# Week 1 execution status

Started 18 October 2026 on `feat/week-1-foundation`. This is the plan's local
technical-spike path. W0 partner commitments are not recorded; the pilot clock and
partner-specific work have not started.

| Week 1 deliverable | Repository evidence | Status |
|---|---|---|
| Selected routes and explicit invariants | `connectors/*/{route,consumer-contract}.json`, ADR 0001, corpus assertions | Synthetic selections ready; partner selection/approval pending |
| Locality, retention, signature and deployment boundary | ADR 0001; bounded local redacted store and loopback/container profile | Local decisions implemented; partner agreements pending |
| Sized Weeks 1–6 work by owner | `week-1-capacity.md` | Preliminary estimates; real owner confirmation pending |
| Independent taxonomy and oracle seed | 21 labeled cases, four authored goldens, Rust oracle, consumer schemas | Ready for local technical work; no independent holdout or real events |
| Repository/build/format/lint/locks/scanning/CI | Go/Rust workspace, Makefile, hashed Python lock, image lock, GitHub workflow | Native checks implemented; CI has not been run remotely |
| Seven primary versioned wire schemas | `schemas/v1`, unsigned examples, schema validator | Defined and validated; service-side semantic release gates remain later work |
| Local infrastructure and simulator/health/metrics | Compose/Envoy config, Go simulator/collector, content-addressed filesystem store | Native service and Compose configuration tested; complete Linux startup pending |
| Compatibility record | `version-compatibility.md` | Native tools tested; Linux image availability verified; Wasmtime/ABI pin pending Week 2 |

## Exit-check evidence and limits

Another developer can launch the native simulator and collect a redacted request
with `make dev` / `make smoke`, without provider or cloud accounts. Each of Stripe,
Shopify, NetSuite and generic JSON has a repairable synthetic move and an unsupported
required deletion. Additional generic cases cover the full Week 1 mutation taxonomy.
The Rust oracle checks four golden repairs without calling a synthesizer. The
corpus has zero customer events and zero held-out events; it is explicitly ineligible
as production release evidence.

The collector persists only redacted structure, marks `authoritative: false`, records
`unverified_fixture` signature status and reports semantic verification unavailable.
Every property name and scalar value is masked. This intentionally lacks useful
field-level drift evidence until a reviewed export policy is implemented. No endpoint
can infer or promote a captured sample to an authoritative consumer contract.

Verification covers Go race tests, bounded/duplicate-key parsing, exact numbers,
missing/null, header/query/key/value redaction, spoofed tenant headers, filesystem
integrity/quota/retention, Rust oracle adversarial tests, JSON schema negative tests,
fixture hashes and target-schema compatibility.

Recorded on macOS ARM64, 18 October 2026:

| Check | Result |
|---|---|
| `make check build` | Passed: formatting/static checks, schema/provenance validation, Go race tests, Rust tests, golden oracle, binaries |
| Native HTTP smoke on loopback port 18081 | Passed: 12 captures across all four families, duplicate-key rejection, 401/429 simulation and metrics |
| govulncheck 1.9.0 | Passed: no vulnerabilities found |
| cargo-audit 0.22.2 | Passed against 1,338 RustSec advisories; no reported vulnerabilities |
| Compose 5.6.0 `config --quiet` with committed image lock | Passed |
| Full Compose startup, Envoy config execution, Linux oracle container | Not run: Docker engine unavailable |
| Remote CI | Not run |

The full Week 1 exit remains open until a Docker-equipped Linux/desktop environment
passes `make compose-check up` and the Envoy smoke command. Native success does not
qualify Envoy, JetStream, PostgreSQL, ext_proc or Wasmtime. The committed CI workflow
executes this full path; no remote CI success is claimed.

## Inputs still required

| Input | Responsible role | Effect |
|---|---|---|
| W0 interview evidence, ≥2 signed LOIs and named Stripe invariant owner | Founder/product | Required before pilot clock/onboarding |
| Exact partner event families, operations and provider releases | Product/integration + partner owner | Replace synthetic route decisions only after approval |
| Payload locality/retention, field export policy and consumer trust agreement | Partner owner/platform | Required before partner capture or transformed delivery |
| Stripe/Shopify test accounts; NetSuite read-only sandbox authorization | Integration + partner owner | Live signature and connector qualification |
| Named engineering owners and confirmed estimates | Technical lead | Re-baseline four-person schedule at M0 |

The next technical work is the Week 2 no-op/guarded-move Rust core, concrete Wasm
ABI, Wasmtime/ext_proc integration, raw-signature fixtures and durability crash
matrix. None of these future behaviors is implied by the current schema files.
