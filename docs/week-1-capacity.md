# Preliminary Weeks 1–6 sizing

23 October update: the [M0 closure tracker](m0-closure.md) still awaits individual
owner confirmations. [Week 3 technical work](week-3.md) has started on synthetic
fixtures; the allocations below remain planning estimates, not accepted staffing,
recorded engineer-days or a re-baselined delivery commitment.

Planning estimates, not recorded work by a staffed four-person team. The technical
lead and each real owner must confirm these estimates before starting the pilot
clock. One engineer has about 3.5 effective engineer-days/week at 70% allocation;
the plan reserves approximately 2 of the team's 14 days each week.

| Week | Product/full-stack | Compiler/verification | Runtime/platform | Backend/integration | Planned total |
|---|---|---|---|---|---|
| 1 | 2: synthetic route decision record, discovery gaps, exit evidence | 3: schemas, invariants, taxonomy/oracle seed | 3: toolchains, Linux images, Envoy, capture limits | 3: simulator, redacted store, CI/health metrics | 11 |
| 2 | 2: spike report and scope re-baseline | 3: no-op/move core and ABI shape | 3.5: ext_proc + Wasmtime, latency/body matrix | 3.5: signatures, object/ledger crash windows | 12 |
| 3 | 2.5: incident CLI and labeled-evaluation review | 3.5: supported-schema IR, normalization/detector | 3: capture quality and parser/resource limits | 3: registry persistence, bindings and incident APIs | 12 |
| 4 | 2.5: mappings and evidence review workflow | 3.5: finite DSL/search, independent oracle | 3: offline build and native/Wasm equivalence | 3: fixture integration, manifest/policy interfaces | 12 |
| 5 | 2.5: approval CLI and staging workflow | 3: replay isolation/holdout reports | 3.5: cached artifacts, crash recovery, staging | 3: leases/outbox, scoped approval and immutable tuples | 12 |
| 6 | 3: reference demo and commercial gate evidence | 3: full-loop replay and unsafe cases | 3.5: canary, rollback and shadow-install qualification | 2.5: signed Stripe flow; other fixture packs only if reserve allows | 12 |

The Week 1 estimate leaves three engineer-days before the team's 14-day capacity,
including its normal two-day reserve. The other weeks use the 12-day planning cap.
All numbers are preliminary and will change after the Week 2 feasibility report.

No partner onboarding, interviews, LOIs, live Stripe access, Shopify access or
NetSuite authorization is assumed complete. Product work without those inputs stays
on local workflow/specification and access tracking. A solo owner cannot execute
this four-person allocation on the proposed six-week schedule: use the original
plan's reduced-scope 16–24+ week re-estimation rule.

Likely pressure points: Week 2 runtime/signature/durability spikes and Week 4
compiler/oracle work. Apply the original descope order before expanding the cap:
GitHub integration, Proxy-Wasm comparison, console polish, notification integration,
then recorded product-approved connector cuts. Safety gates remain prerequisites.
