# Product Requirements Document
# Self-Healing B2B API Mesh & Schema-Drift Compiler

**Status:** Proposed product specification; implementation has not started.  
**Prepared:** 17 October 2026.  
**Primary source:** [High-Moat Startup Blueprints 2026](High-Moat%20Startup%20Blueprints%202026%20%281%29.pdf), Blueprint #1 on pages 2 and 5, and execution guidance on page 8.  
**Companion documents:** [Architecture](architecture.md) · [Implementation plan](implementation-plan.md).

**Navigate:** [Scope](#3-goals-exclusions-and-release-scope) · [Features](#5-feature-catalog-and-acceptance-criteria) · [Repair eligibility](#6-repair-eligibility-and-supported-transformations) · [User journeys](#7-main-user-journeys) · [SLOs](#9-nonfunctional-requirements-and-slo-candidates) · [Release acceptance](#12-release-acceptance-and-unresolved-decisions).

## 1. Product definition

Build an integration reliability layer that observes third-party API traffic, detects contract changes, proposes narrowly scoped compatibility adapters, proves those adapters against explicit contracts and test evidence, and deploys approved repairs with controlled rollout and rollback.

The product preserves a customer's established consumer contract while an upstream provider changes its representation. Its main output is a verified compatibility release, accompanied by an explanation, tests, operational evidence, and an optional pull request that lets the customer move the permanent fix into source control.

An **API mesh** here means a managed inventory of partner-facing routes and their compatibility policies. It does not require replacing a customer's service mesh or routing all internal traffic through the product.

“Self-healing” means executing an authorized, verified repair within a defined policy. It does not mean inventing missing business facts, guessing payment semantics, changing credentials, or fixing every upstream outage.

### 1.1 Source interpretation

| Blueprint direction | Product decision |
|---|---|
| Traffic mirroring through eBPF or Envoy | Start with explicit Envoy/application-boundary integration; add Linux eBPF observation after validating a compatibility matrix. |
| AST-aware schema comparison and synthesis | Use a typed schema intermediate representation, a finite transformation language, deterministic synthesis, and optional model-generated candidates. |
| Rust/Wasm adapters | Compile verified transformations to Wasm; use a local Wasmtime execution service for the initial production path. Evaluate an Envoy Proxy-Wasm profile separately. |
| Replay 1,000 shadow requests | Require representative, held-out evidence and semantic assertions; 1,000 executions alone are insufficient. |
| Hot patch plus Git PR | Versioned canaries and rollback, with an optional GitHub App creating a reviewable PR. |
| Stripe, Shopify, NetSuite in six weeks | Demonstrate two webhook packs and a NetSuite REST fixture pack in six weeks; live NetSuite qualification belongs to the pilot. |
| Usage fee per healed call | Test a platform fee plus protected-traffic allowance; report healed calls separately to avoid incentives to make unnecessary transformations. |

The PDF's market percentages, change-frequency estimates, zero-latency wording, and ecosystem attributions are source claims, not validated product facts. Discovery and benchmarks must establish our own evidence. All numerical targets below are proposed acceptance criteria, not achieved performance or contractual commitments.

## 2. Problem, customer, and value

Third-party changes can cause obvious failures, such as a missing required property, or silent failures, such as interpreting a changed unit incorrectly. Teams often discover the problem through customer complaints, investigate sparse logs, patch custom integration code, and repeat the work for another provider.

### 2.1 Initial customer profile

- Mid-market fintech, commerce, logistics, and SaaS businesses with approximately 20 or more external integrations and identifiable integration ownership.
- Teams with repeated JSON API or webhook incidents, permission to deploy a gateway or local collector, and access to sanitized historical failures.
- First pilot workloads: noncritical shipment/order metadata and nonfinancial fields in signed webhook envelopes. Payment execution, clinical decisions, and ledger mutation are excluded from automatic repair.
- Buyer: VP Engineering or platform engineering lead. Daily users: integration engineers and SREs. Security reviewers approve capture boundaries and deployment permissions.

| Persona | Job to be done | Required outcome |
|---|---|---|
| Integration engineer | Find the exact contract mismatch and obtain a trustworthy fix | Before/after diff, typed adapter, failing/passing fixtures, reproducible evidence |
| SRE/on-call engineer | Restore compatibility without widening the incident | Scoped rollout, health indicators, pause, rollback, quarantine, runbook |
| Platform lead | Apply consistent protection across integrations | Route inventory, deployment policy, ownership, fleet status, usage |
| Security administrator | Limit exposure and unauthorized changes | Local redaction, secret references, tenant isolation, RBAC, audit export |
| Engineering leader | Evaluate operational value | Incident reduction, restoration time, reviewed repairs, effort saved, cost |

### 2.2 Value hypothesis and validation

The initial hypothesis is that a recurring subset of integration incidents consists of representational changes with an objectively verifiable mapping. Validate this through at least three design partners, ten incident interviews, and a labeled fixture corpus before expanding protocol coverage. Compare with maintaining provider API versions, contract tests, existing gateway rules, and manual integration patches. If most incidents require business judgment, prioritize diagnosis and reviewed fixes instead of broad autonomy.

## 3. Goals, exclusions, and release scope

### 3.1 Product goals

1. Detect breaking changes in observed, supported traffic before they cause a long-running integration failure.
2. Restore compatibility for a bounded class of structural changes with clear evidence and a reversible release.
3. Make unsupported or ambiguous changes explicit and actionable.
4. Keep payloads, secrets, and deployment authority inside the customer's agreed trust boundary.
5. Reduce repeat repair work through reusable mutation patterns, regression fixtures, and source-controlled adapters.

### 3.2 Explicit exclusions from the initial release

- Generic iPaaS workflow building, ETL, a universal canonical business data model, or replacing all vendor SDKs.
- Automatic payment/refund creation, monetary conversion, clinical interpretation, permission changes, or database correction.
- Reconstructing deleted data, guessing identifiers, inferring a new enum's business meaning, or changing units without an owner-approved rule.
- Transparent inspection of arbitrary encrypted traffic, certificate pinning bypass, or universal eBPF support.
- SOAP/XML transformations, arbitrary GraphQL rewrites, streaming gRPC repairs, WebSockets, multipart/file transformations, and unbounded payloads.
- Arbitrary customer code execution or model-written code deployed directly to a gateway.
- A claim of global exactly-once processing or a guarantee that rollback reverses downstream side effects.

### 3.3 Release definitions

| Release | Intended outcome | Boundary |
|---|---|---|
| **R0: six-week prototype** | A working observe → detect → compile → replay → manually deploy → rollback loop | Local/test environment; deterministic fixtures; limited partner shadow collection; no autonomous production deployment |
| **R1: weeks 7–12 pilot** | Customer-hosted data plane with reviewed repairs, operational controls, and measurable value | Three design partners; qualified routes only; all new repair candidates require a human approval |
| **R2: post-pilot expansion** | Explicitly opted-in, low-risk automation and broader enterprise/protocol support | Promotion depends on safety evidence and demand; no fixed delivery date |

### 3.4 Protocol and connector coverage

| Surface | R0 | R1 | R2 |
|---|---|---|---|
| HTTP/1.1 and HTTP/2 with finite UTF-8 JSON bodies | Explicit gateway routes and fixtures | Qualified inbound/outbound routes | Larger certified size classes |
| OpenAPI / JSON Schema | Declared subset of OpenAPI 3.1 + JSON Schema 2020-12; normalized import of common OpenAPI 3.0 constructs | Expanded subset with conformance fixtures | Further dialects only with compatibility tests |
| Stripe | Signed snapshot-webhook fixtures and test endpoint | Selected snapshot event families and pinned endpoint versions | Thin-event fetch workflows as an explicit connector feature |
| Shopify | Signed JSON webhook fixtures and development-store tests | Selected order/product topics, pinned versions | Additional topics and declared GraphQL operations |
| NetSuite | REST JSON fixture pack and read-only simulator | Selected REST record reads in a separately authorized sandbox | Explicitly reviewed write support and custom integration events |
| Generic partner webhooks | Local envelope format | Documented HMAC or mTLS authentication pack | Additional signature mechanisms |
| gRPC / Protobuf | Preserve/pass through; no schema repair | Inventory only when descriptors are supplied | Unary descriptor-aware observation and repair qualification |
| eBPF capture | Architecture and Linux prerequisite assessment only | Optional lab experiment, not a dependency | Supported kernel/TLS/library combinations released individually |

Provider packs are versioned software: signature handling, event identity, schemas, fixtures, API version extraction, rate-limit classification, and documented limits. “Supports Stripe” never means “supports every Stripe API.” Stripe distinguishes snapshot and thin events; the initial product intentionally limits itself to snapshot payloads. See [Stripe webhook documentation](https://docs.stripe.com/webhooks).

## 4. Product operating modes

| Mode | What happens | Who may enable it |
|---|---|---|
| Observe | Capture approved metadata/samples and report drift; delivery bytes remain unchanged | Integration engineer with route access |
| Recommend | Add candidate adapters and replay reports; no live changes | Integration engineer |
| Approved repair | Deploy the exact approved artifact to a scoped canary, then promote if gates pass | Authorized approver; operator may execute an already approved release |
| Bounded automatic repair | Deploy only preauthorized, low-risk candidates meeting all release gates | Tenant administrator and route owner; R2 only |
| Paused/quarantined | Stop promotions or hold incompatible webhook events according to route policy | Operator, approver, or automatic circuit breaker |

Approval binds the artifact hash, source and destination contract hashes, test evidence, policy version, route, environment, and expiry. Editing any bound input invalidates the approval. Enabling automatic mode does not waive these gates.

## 5. Feature catalog and acceptance criteria

Feature identifiers are shared with the architecture and implementation plan. R0 entries describe prototype capability; operational hardening is still required for R1.

| ID / feature | Workflow and behavior | Release | Acceptance criterion |
|---|---|---|---|
| **F01 Tenant, project, environment, and roles** | Create an organization, project, staging/production boundaries, memberships, and service identities. Separate viewing, authoring, approving, operating, and administration. | R0 local identity; R1 OIDC/RBAC | Cross-tenant and cross-environment reads/writes are denied; an author cannot self-approve production repairs by default. |
| **F02 Connector onboarding and route inventory** | Register provider, owner, exact host/method/path/event family, credentials by reference, contract, size limits, and delivery mode; run a connectivity/signature check. | R0 → R1 | A route reaches “observe-ready” only after the deployment and trust-boundary checks pass; unknown routes are visible as unsupported. |
| **F03 Traffic capture and privacy controls** | Record status/latency/schema fingerprints and approved payload samples at the local boundary. Redact before export, use bounded queues, and display sample/drop coverage. | R0 → R1 | Capture failure does not block a pass-through route; sensitive canary strings never appear in exported telemetry; partial bodies cannot become replay evidence. |
| **F04 Contract registry and inference** | Import declared contracts, infer observed shapes, version and compare them, and let owners publish a consumer contract. Inferred shapes remain hypotheses. | R0 | Nullable, missing, empty, array, union, and unknown-field cases remain distinct; inferred absence never silently changes requiredness. |
| **F05 Drift detection and incident classification** | Compare contract and observed shape; correlate schema mismatches with route/version/status. Separate structural incidents from authentication, throttling, availability, and business errors. | R0 → R1 | Labeled synthetic changes produce the expected class; 401/403/429 and ordinary declines never automatically produce a schema patch. |
| **F06 Impact analysis and incident workspace** | Group repeated observations, identify affected consumers/routes, show first seen, evidence coverage, severity, owner, and actions. | R0 CLI; R1 UI | One incident groups duplicate evidence without hiding distinct provider versions; estimated affected traffic is labeled as estimated when sampled. |
| **F07 Deterministic repair synthesis** | Enumerate typed operations from approved rules and schemas, reject ambiguity, produce a reproducible adapter and an explanation. | R0 | Supported unambiguous fixtures compile; ambiguous rename, lossy cast, missing-source, and collision cases are rejected. |
| **F08 Optional AI-assisted proposals** | Send only permitted schemas and sanitized evidence to a configurable model; accept a constrained DSL candidate and rationale. Apply identical verification gates. | R2 | Disabling the model preserves the deterministic pipeline; hostile payload instructions cannot trigger tools, secret access, or deployment. |
| **F09 Contract and semantic invariant policies** | Owners define preserved fields, identifier rules, decimal/unit constraints, allowed operations, required evidence, and approval requirements. | R0 → R1 | A candidate that changes a protected invariant is rejected even if its output schema validates. |
| **F10 Isolated build, replay, and verification** | Compile an immutable artifact, execute historical and held-out cases against a contract oracle, run negative/property tests, and produce signed evidence. | R0 → R1 | No production destinations are reachable from replay; any required failure blocks eligibility; evidence identifies runtime/toolchain and fixture hashes. |
| **F11 Approval and policy engine** | Evaluate candidate risk and evidence, gather a scoped approval, and recheck freshness immediately before release. | R0 manual; R1 enforced | Unauthorized, expired, stale, or edited approvals cannot deploy; denials carry machine-readable reason codes. |
| **F12 Runtime adapters and controlled rollout** | Stage artifact, warm local executor, pin version per request/event, start canary, observe health, and promote. | R0 test; R1 production pilot | Every transformed delivery identifies one adapter hash; rollouts never duplicate live writes for comparison. |
| **F13 Rollback, kill switch, and quarantine** | Halt rollout, restore the previous version or hold affected events, and explain recovery options. | R0 → R1 | Kill switch and rollback work during control-plane outage via a local operator path; a known-incompatible prior version causes quarantine instead of an assumed recovery. |
| **F14 Webhook authenticity and durable delivery** | Verify original bytes, durably accept eligible asynchronous events, deduplicate provider deliveries, transform internally, and deliver a new authenticated envelope. | R0 local; R1 hardened | Invalid signatures are rejected; no success acknowledgement precedes durable acceptance; transformed content is never presented as provider-signed original content. |
| **F15 Retry, throttling, and auth health** | Honor declared quotas and Retry-After, bound retries, preserve idempotency keys, report credential expiry, and use a configured refresh flow only. | R0 classification; R1 bounded controls | No blind retry of an uncertain non-idempotent write; no permission expansion or synthetic credential repair. |
| **F16 Pagination and sequence diagnostics** | Observe cursor/offset fields, checkpoint sequences, detect loops/duplicates/missing progress, and offer a reviewed protocol mapping when evidence exists. | R1 diagnostics; R2 transformations | A synthetic repeated cursor or missing-page condition is flagged; single-payload tests cannot qualify a sequence-level repair. |
| **F17 GitHub and permanent-fix workflow** | Optionally create a branch/PR with adapter DSL, generated wrapper, safe fixtures, evidence manifest, and rollout linkage; track merge status. | R1 | Repository permission is scoped; PR content contains no secrets/raw production payloads; merge never automatically retires the live shim. |
| **F18 Console, CLI, API, and notifications** | Expose inventory, incidents, candidate diff, replay report, approvals, rollout, usage, and settings; send deduplicated alerts to configured destinations. | R0 CLI/API; R1 UI/alerts | The complete repair journey is available by API; UI distinguishes unknown, stale, dropped, healthy, and repaired states. |
| **F19 Audit, export, retention, and deletion** | Track configuration and release actions, export evidence, enforce retention, and remove tenant data using an auditable deletion job. | R0 metadata; R1 hardened | All production changes link to an actor and evidence; deletion includes objects, indexes, queues, and documented backup expiry. |
| **F20 Observability, resilience, and support operations** | Surface latency, mismatch rate, queue lag, adapter failures, capture gaps, runtime version, and control-plane connectivity; include recovery runbooks. | R0 metrics; R1 SLO operations | Injected outages exercise the stated failure policy; a disconnected or under-sampled route cannot appear healthy. |
| **F21 Usage, quotas, and commercial reporting** | Count protected calls, actual transformed calls, replay jobs, storage, and usage by route; enforce capture/build quotas separately from live forwarding. | R1 | Deduplication prevents delivery retries becoming multiple unique-event charges; quota exhaustion never silently drops an already accepted webhook. |
| **F22 Mutation pattern library and regression corpus** | Store reviewed structural patterns and outcome labels; reuse templates within a tenant; share sanitized patterns only under explicit consent. | R0 internal fixtures; R1 tenant library; R2 shared corpus | A reused pattern still requires local contracts and verification; no tenant payloads or identifiers leak into another tenant's suggestions. |
| **F23 eBPF observation agent** | Linux agent attaches only to qualified processes/hooks, reconstructs supported messages, reports gaps, and emits local observations. | R2 | Kernel/library/build matrix, CPU overhead, truncation, HTTP/2 correlation, and plaintext visibility are measured; unsupported binaries are explicit. |
| **F24 Extended contracts and protocols** | Add unary Protobuf descriptor compatibility, declared GraphQL operations, and reviewed request-side transformations as separate capabilities. | R2 | Each protocol has an independent parser, semantic rules, fixtures, rollout policy, and qualification suite. |
| **F25 Private deployment and enterprise governance** | Add private control plane, SSO federation/SCIM, network isolation, residency controls, dedicated tenancy, and customer-managed key options. | R1 customer-hosted data plane; remaining R2 | Deployment mode documents egress and recovery; identity revocation and tenant-bound artifacts are tested end to end. |
| **F26 Adapter lifecycle and shim retirement** | Detect stale/unneeded repairs, enforce review dates, verify the customer's permanent fix, and remove the shim through a no-op canary. | R1 | Expiry freezes new promotion and alerts owners; active adapters are never removed merely because a timer elapsed. |

## 6. Repair eligibility and supported transformations

### 6.1 Transformation semantics

Use a declarative, versioned DSL with JSON Pointer paths. Initial operations are `copy`, `move`, `remove` of explicitly unconsumed fields, `assert`, and guarded composition. Later reviewed operations include exact numeric-string parsing, explicit enum tables, and flatten/nest mappings with collision checks.

Every operation must declare source and destination type expectations, missing-value behavior, conflict behavior, and its evidence. The compiler preserves missing versus null, does not coerce identifiers to numbers, and never performs floating-point arithmetic on monetary values. A route uses one composed adapter, rather than an unbounded stack of historical shims.

| Change | Default classification | Permitted behavior |
|---|---|---|
| New optional field | Usually nonbreaking; consumer contract decides | Observe; leave untouched unless an explicit closed-schema policy requires removal |
| Field moved/renamed with approved semantic equivalence | Repair candidate | Guarded copy/move, with no destination collision and a preserved-fields assertion |
| New required upstream request property | Potentially breaking | Block/recommend; only use an explicitly supplied constant or mapped existing value |
| Required field removed with no equivalent source | Unsupported | Alert or quarantine; never manufacture the value |
| Numeric string replacing integer | Potentially representational | Reviewed exact parse with bounds and round-trip checks; never for opaque IDs |
| Enum addition | Unknown business meaning | Preserve if consumer permits it; otherwise escalate for a mapping decision |
| Unit/currency/time-zone change | Semantic risk | Diagnose; owner-defined rule and additional tests; excluded from automatic repair |
| Authentication scheme/permission change | Operational/security incident | Alert and configured credential maintenance; no schema synthesis |
| Rate-limit reduction | Operational incident | Bounded scheduler/backoff configuration within existing policy |
| Pagination token/termination change | Sequence-level protocol change | Diagnostics first; independently verified sequence adapter in R2 |
| A structurally valid but wrong business value | Semantic incident | Detect only through explicit invariants or downstream feedback; no universal guarantee |

### 6.2 Risk and approval tiers

- **Tier A:** Predeclared, noncritical representational mappings with unique evidence. R1 still requires approval. R2 may automate within a route's explicit allowlist.
- **Tier B:** Lossless casts, enum tables, multi-field mapping, or ambiguous alternatives resolved by a human. Human approval is always required.
- **Tier C:** Money, identity, permissions, legal/clinical meaning, business-state transitions, or missing information. Diagnose and prepare evidence; do not automatically deploy a repair.

A model confidence score cannot promote a tier or substitute for an assertion. A lack of failing examples is not proof of semantic correctness.

## 7. Main user journeys

### 7.1 Onboard a route

1. An administrator creates project/environment membership and chooses the deployment/data residency mode.
2. An engineer selects a provider pack, specifies exact routes/event families, imports a consumer contract, and identifies protected fields.
3. The local agent establishes an authenticated control connection and validates capture, signature handling, storage, destination allowlists, and the provider version.
4. The engineer enables observe mode and sees capture coverage, size exclusions, signature failures, and inferred shapes.
5. After reviewing evidence, an approver enables recommend or approved-repair mode. No baseline is silently accepted as correct merely because traffic was observed.

### 7.2 Repair a provider field move

Illustrative synthetic payloads: the consumer expects `shipment.tracking_number`, but the provider now emits `shipment.tracking.number`. The approved contract annotation says these represent the same noncritical tracking identifier.

1. Detection groups the mismatch by route, event type, and provider version.
2. Synthesis proposes a guarded move and refuses to overwrite an existing conflicting destination.
3. Verification checks the tracking identifier is unchanged, unrelated fields remain unchanged, old and new shapes both behave as intended, and malformed/ambiguous cases fail predictably.
4. The owner reviews the diff and evidence, then approves a canary for this route only.
5. The deployment promotes after minimum traffic, time, and safety checks; a failing assertion causes immediate halt and quarantine/rollback.
6. The optional GitHub integration opens a permanent-fix PR. After the consumer deploys that fix and passes a no-op test, the shim is retired.

### 7.3 Handle an unsafe or unsupported incident

A provider removes `currency`, changes a payment amount's unit, or rejects a request for authorization reasons. The incident shows why the repair is ineligible. Operators can pause affected routes, hold already accepted webhooks, or use the route's predefined pass-through/error behavior. The product does not guess missing semantics. The owner resolves the underlying contract and provides new evidence.

### 7.4 Work during a control-plane outage

Existing routes continue using the locally cached, verified configuration and artifacts. New repairs and promotions stop. Capture queues remain bounded, accepted webhook delivery continues within available local dependencies, and operators can trigger a local kill switch. Reconnection reconciles desired versus observed versions before resuming promotion.

## 8. UX and product surfaces

| Surface | Required information and actions |
|---|---|
| Overview | Protected routes, active incidents, delivery lag, capture coverage, rollout health, stale agents |
| Integration detail | Owner, provider/version, endpoints, contract history, trust mode, quotas, operational health |
| Incident detail | Evidence timeline, structural delta, classification, affected contracts, unsupported reasons |
| Candidate review | Before/after examples, operation list, protected-field checks, coverage, failed cases, risk tier |
| Replay report | Dataset provenance, holdout results, assertions, runtime/build identity, deterministic output hashes |
| Deployment | Approved digest, current/candidate versions, traffic allocation, gating metrics, pause/rollback |
| Delivery queue | Accepted/pending/failed events, duplicate status, attempt history, safe redelivery controls |
| Settings | Identities, policies, destinations, retention, model consent, repository permissions, deployment mode |

Interfaces must use keyboard-accessible controls, readable diff colors, clear timestamps/time zones, and confirmation dialogs for production mutations. Mask payload values by default. Show “not enough evidence” instead of a reassuring success state for sparse traffic. Production actions require an explicit environment indicator.

## 9. Nonfunctional requirements and SLO candidates

Targets apply to qualified finite JSON routes, measured in a documented test environment. External provider downtime is excluded from product availability but displayed separately.

| Property | R1 acceptance target and measurement |
|---|---|
| Gateway processing overhead | Added p95 ≤ 5 ms and p99 ≤ 10 ms for warm 32 KiB JSON transformations at 500 requests/s per qualified 4-vCPU/8-GiB gateway/executor pair; report CPU and baseline. Signature ingress/storage latency is measured separately. |
| Larger payload envelope | Default maximum transformed JSON body 256 KiB, maximum depth 64; benchmark boundary cases separately and refuse unsupported cases according to route policy. |
| Observe-mode availability | Failure of sample export never blocks forwarding. Added observation overhead measured and published; no “zero latency” promise. |
| Durable webhook acceptance | p95 ≤ 1 second after receipt of the complete supported body in a healthy regional deployment; return success only after durable persistence. |
| Data-plane availability | Initial monthly objective 99.9% for valid supported requests; separate synchronous forwarding and durable webhook-acceptance indicators. |
| Control-plane availability | Initial monthly objective 99.5%; an outage cannot stop already deployed adapters. |
| Detection delay | p95 ≤ 60 seconds from receipt of a captured qualifying anomaly to an incident; detection of unsampled traffic is not promised. |
| Verification turnaround | p95 ≤ 5 minutes for a warm build and 1,000 small fixture executions, excluding human approval and corpus acquisition. |
| Rollback | ≤ 60 seconds to stop new assignments to an adapter on reachable healthy nodes; disconnected nodes are reported explicitly. |
| Accepted-event durability | No acknowledged event loss under the tested single-node/AZ failure model; regional disaster RPO is documented separately. |
| Disaster recovery | Metadata RPO ≤ 15 minutes, regional service RTO ≤ 4 hours for pilot backup restoration; validate by drill before commitment. |
| Isolation | No cross-tenant access in the tenant boundary test suite; tenant scope enforced across storage, APIs, workers, and artifacts. |
| Auditability | Every production release has immutable artifact identity, evidence, authorization, and observed deployment status. |

## 10. Data handling and security requirements

- Payload storage stays in the customer data plane by default. A hosted control plane receives approved schemas, redacted evidence, metrics, and metadata only.
- Telemetry payload persistence is opt-in, encrypted, and defaults to 24 hours. Sanitized replay fixtures default to seven days; incident metadata to 30 days; audit metadata to 180 days. Tenant policy may shorten these limits.
- Operational webhook payloads are distinct from telemetry samples: accepting a webhook requires durable storage. Retain pending/DLQ bodies for up to seven days by default, alert before expiry, and purge successful bodies within 24 hours. A customer that prohibits storage must use observe or synchronous mode.
- Never export authorization headers, cookies, API keys, or webhook signing secrets. Keep provider secrets in a local secret manager and reference them by identifier.
- Preserve original bytes for signature verification. A transformed body has a new mesh identity and signature; it does not inherit a provider signature. Shopify documents raw-body HMAC verification in its [delivery verification guide](https://shopify.dev/docs/apps/build/webhooks/verify-deliveries).
- Role separation, authenticated service connections, authenticated artifacts, bounded execution, allowlisted destinations, and replay network isolation are release requirements.
- Raw evidence and customer contracts are untrusted inputs. Treat embedded instructions as data. External model use requires tenant consent and explicit export policy; no model is in the live request path.
- Compliance certifications are not claimed by this specification. Enterprise questionnaires and required audits become separate release deliverables where customers require them.

## 11. Measurement and business model

### 11.1 Product metrics

| Metric | Definition | Initial validation goal |
|---|---|---|
| Time to first observed route | Start of install to valid capture and route ownership | ≤ one working day for a qualified deployment |
| Detection precision | Correctly labeled actionable drift incidents / reviewed drift incidents | ≥ 95% on the labeled pilot evaluation set |
| Detection recall | Detected supported injected mutations / supported injected mutations | ≥ 95%; report unsupported classes separately |
| Eligible repair success | Correct verified repairs / known repairable labeled mutations | ≥ 90% for the finite declared operation set |
| Unsafe release count | Deployed repairs violating a protected invariant | Zero; any occurrence freezes automatic-release work |
| Restoration time | First actionable observation to validated restored compatibility | ≥ 50% reduction versus each partner's comparable historical cases |
| Evidence coverage | Unique cases and semantic variants tested per candidate | All declared variants represented; no count-only pass |
| Pilot conversion | Partners paying after a measured shadow/approved-repair period | At least two of three, subject to discovery |
| Net value | Documented engineering time/incident cost avoided less operating cost | Positive for each renewing customer |

Do not count a request as “healed” just because an adapter ran. Track `transformed`, `contract_valid_after`, and `downstream_accepted` separately; a successful HTTP status is not proof of a correct business outcome. Unsupported incidents, quarantined events, false positives, and unknown outcomes remain visible.

### 11.2 Commercial hypothesis

Offer a platform fee plus protected-call volume and included retention/replay allowances, with enterprise charges for private deployment and support. Keep exact pricing open until measured pilot costs and buyer interviews exist. A free local evaluator and a public synthetic mutation benchmark can support adoption; proprietary value comes from trustworthy verification, operational integration, and a consented pattern library.

## 12. Release acceptance and unresolved decisions

### 12.1 R0 completion

The reference demo must ingest a signed synthetic webhook, detect a field move, compile a deterministic Wasm adapter, reject unsafe candidates, replay fixtures without external side effects, manually apply the approved artifact in staging, and roll back. Publish benchmark methodology and the supported-subset list. NetSuite may remain fixture-based at this gate.

### 12.2 R1 pilot completion

All R1 feature slices in the implementation plan must pass their acceptance checks. Each pilot runs at least 14 days in shadow/recommend mode before a production repair. Tenant isolation, signature handling, durable delivery, idempotency, runtime limits, release freshness, rollback, and backup restoration must pass fault-injection tests. At least one qualified connector per partner must show a reviewed end-to-end repair; synthetic mutation drills may supplement naturally occurring incidents and must be labeled.

### 12.3 Decisions to settle through discovery or engineering spikes

| Decision | Default assumption | Owner / deadline |
|---|---|---|
| First deployment topology | Hosted control metadata + customer-hosted data plane | Platform lead, week 1 |
| First route/event families | Two webhook families and one read-only NetSuite REST operation, using available partner evidence | Product + integration lead, week 1 |
| Production Wasm host | Local Wasmtime executor behind Envoy external processing | Runtime lead, validated by week 2 |
| Minimum evidence for low-volume routes | Reviewed explicit waiver; automatic release stays disabled | Verification lead + route owner, week 4 |
| Additional model/provider | None required for R0/R1; evaluate against held-out cases before enabling | Compiler lead, after pilot |
| Long-term pricing | Platform + protected volume | Founder/product lead, weeks 9–12 |
| NetSuite sandbox and custom event emitter | Separate sandbox authorization; generic webhooks only if a customer supplies an emitter | Integration lead, week 2 |

These assumptions allow engineering to start. A change to risk scope, supported protocols, or data residency requires updating this PRD, the architecture, and the linked implementation milestone together.
