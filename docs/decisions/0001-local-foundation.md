# ADR 0001: Synthetic, observe-only Week 1 foundation

Status: accepted for the local technical spike, 18 October 2026. Partner-specific
decisions are pending W0 evidence; this record does not approve a pilot.

The repository began with the PRD, architecture and implementation plan only.
There is no recorded route shortlist, signed LOI, named partner invariant owner or
authorized provider account. Follow the plan's W0 fallback: build the technical
foundation with synthetic fixtures while discovery remains outside the codebase.

## Route and consumer decisions

| Fixture family | Selected local operation/family | Allowed synthetic equivalence | Protected invariants |
|---|---|---|---|
| Stripe, first reference path | `customer.updated`, snapshot-like local JSON | `/data/object/metadata/presentation/label` → `/data/object/metadata/display_label` | Event ID/type and customer ID unchanged |
| Shopify | `products/update` | `/details/vendor` → `/vendor` | Exact product ID and business status unchanged |
| NetSuite | Read-only `GET inventoryItem/{id}` | `/presentation/displayName` → `/displayName` | Item ID and SKU unchanged |
| Generic JSON | `shipment.updated` | `/shipment/tracking/number` → `/shipment/tracking_number` | Event/shipment IDs, status and weight unchanged |

These are invented test mutations, including intentionally altered provider shapes.
`provider_version: fixture-v1` identifies a local fixture format. It is not a Stripe,
Shopify or NetSuite API release. Live account versions and event schemas must be
selected by owners and qualified separately. The metadata nesting in the Stripe
mutation is intentionally synthetic; it does not claim Stripe permits nested
metadata in a live object.

The display/tracking strings must be preserved exactly. A destination collision,
deleted identity, missing source, null/string mismatch or semantic change is
unsupported. The corpus contains a >2^53 numeric identifier to expose accidental
rounding. No monetary, identity or business-state transformation is authorized.
Target contracts and golden outputs are authored independently of transformation
code. The oracle checks value preservation, unrelated fields, and source absence.

## Locality, retention and trust

- Only synthetic data may enter this local profile. The filesystem adapter is
  development-only, unencrypted, private to the local OS user and capped at 64 MiB /
  10,000 objects. Its 24-hour TTL since latest capture is swept at startup, on writes and every minute.
- Persist only redacted structural samples. The initial conservative policy masks
  all property names and scalar values, including numeric secrets. It sacrifices
  field-level detection and semantic proof; every observation records that gap.
  A reviewed path/field export policy is future work, not implicit permission.
- No raw provider body, auth/cookie/signature header or URL query is persisted.
  Body limit: 256 KiB; depth: 64; decoded value nodes: 10,000. Malformed, duplicate-key,
  partial, compressed and unsupported-content bodies are rejected as evidence.
- Provider signatures are **unverified**. There is no durable acceptance or business
  delivery endpoint; HTTP 200 from `/observe/...` acknowledges a redacted sample only.
  It must never be registered as a real provider webhook destination.
- Local identity comes from embedded route configuration, not tenant headers.
  This is not production authentication or multi-tenant isolation.
- The native server binds loopback. Compose is an isolated Linux network with
  loopback-published gateway/admin ports. PostgreSQL and NATS are provisioned but
  have no authoritative ledger/outbox application integration yet.
- ext_proc, Wasmtime, original provider signature verification, durable ledger and
  crash-window qualification remain Week 2 spikes. The pilot topology remains
  hosted metadata plus customer-hosted data plane, pending partner confirmation.

Before any partner capture, obtain the invariant owner, exact provider version,
approved field policy, locality/retention decision, signature boundary and deployment
boundary. Before durable mode, implement encryption, quota admission, durable
object-first/ledger commit, retry/dedupe policy and crash-window tests.

Provider references checked for family and trust-boundary context:
[Stripe webhooks](https://docs.stripe.com/webhooks),
[Shopify webhook reference](https://shopify.dev/docs/api/webhooks/latest).
The NetSuite operation is a proposed read-only fixture path; account-specific
availability and REST schema are unverified until sandbox access exists.
