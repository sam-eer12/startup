# ADR 0002: Bounded core Wasm and synthetic acceptance spike

Status: accepted for local feasibility work, 18 October 2026. M0 and production
eligibility remain separate decisions. ADR 0001's prohibition on customer traffic
still applies; the new raw-storage adapter is accessible only from local tests.

The primary runtime remains Envoy ext_proc → Tonic/Prost → Wasmtime. Pin Wasmtime
49.0.3, envoy-types 0.7.7, Tonic 0.14.6 and Prost 0.14.4 in Cargo.lock. The generated
Envoy protocol types avoid a second hand-maintained protobuf definition. Only the
synthetic `/runtime/...` routes use this processor; observation routing is unchanged.
Proxy-Wasm is deferred under descope item 2 rather than introduced as another host.

## Executable core ABI v1

Compile `adapter-wasm` for `wasm32-unknown-unknown`. The module imports nothing:
no WASI, filesystem, clock, randomness, network, environment or signing secrets.
Exports:

| Export | Signature | Contract |
|---|---|---|
| `memory` | linear memory | Host bounds every accessed range |
| `abi_version` | `() -> u32` | Exactly `1` |
| `alloc` | `(length: u32) -> u32` | Zero on invalid length; otherwise a live zeroed allocation |
| `dealloc` | `(pointer: u32, length: u32)` | Host returns the exact original pointer and length once |
| `transform` | `(input_ptr, input_len, output_ptr, output_capacity: u32) -> i32` | Initialized output length; negative means rejection at the ABI boundary |

All pointers are wasm32 byte offsets. The host owns both allocation lifetimes and
requires distinct, non-overlapping buffers. The guest never returns a new pointer.
Input is UTF-8 JSON: `{"abi_version":1,"plan":{"kind":"identity"},"body":"..."}`.
The other plan is `{"kind":"move_string","from":"/source","path":"/target"}`.
Plan configuration comes from trusted local route selection, never client headers.
The Week 4 DSL/compiler is still unimplemented; this is not a general DSL executor.

Output is one of `{"status":"unchanged","body":"..."}`,
`{"status":"transformed","body":"..."}`, or
`{"status":"rejected","code":"..."}`. Rejection contains no payload/path values.
ABI envelopes are capped at 2 MiB to allow JSON string escaping around a 256 KiB
body. Unknown fields/versions, invalid UTF-8/JSON, duplicate object members,
depth >64, >10,000 value nodes and serde_json's reserved number-sentinel property
are rejected. Identity preserves original bytes; move preserves number tokens,
requires an existing string source and object destination parent, and rejects
collisions, missing/null sources and overlapping paths. No parent creation,
type coercion, identity/business-field authorization or schema inference is implied.

Each invocation gets a fresh instance with at most 32 MiB linear memory, one memory,
one instance, one table/10,000 table elements, 50 million fuel and a 10 ms deadline.
A 1 ms epoch ticker interrupts guest execution. The host validates result size,
JSON and unchanged bytes. Both input and output must fit the body budget; a move
that expands beyond it is rejected. ext_proc admits at most 64 streams and 64 executions,
with no waiting admission queue, a 5 s idle-stream bound and 25 ms processing bound.
Blocking work retains its execution permit until it finishes, even on RPC timeout.
The compiled module is reused; mutable guest instances are never pooled.

## Body policy and trust boundary

Only complete, uncompressed `application/json` response bodies on the synthetic
lab routes are transformed. Envoy assembles chunking in BUFFERED mode; STREAMED
and BUFFERED_PARTIAL are rejected. Empty responses pass without a transform.
Trailers/streaming, compressed content, non-200 nonempty responses, malformed JSON,
ambiguous relevant headers and oversized/incomplete bodies are unsupported. Before
moving fields the processor removes content length, ETag and body-digest headers;
Envoy determines the resulting framing. HTTP/2 multiplexing is tested on the actual
Tonic transport; this does not qualify downstream HTTP/2 or gRPC payload repair.
Known Stripe/Shopify/mesh signature headers cause rejection in this lab; the lab
does not authenticate signed provider response bodies.

`internal/trust` uses Stripe's SDK against original bytes before parsing. A single
timestamp and ±5-minute window are required. Shopify uses base64 HMAC-SHA256 of
raw bytes without Stripe's timestamp prefix; live delivery ID/freshness policy is
not qualified. Mutated bytes must fail the original signature.

Mesh delivery uses go-jose 4.1.6, detached compact JWS, EdDSA/Ed25519, standard
base64url payload encoding, protected `alg`/`kid`/versioned `typ`, and an explicitly
configured public key. Verify raw envelope bytes before parsing, then check scope,
audience, time and exact transformed-data digest. Envelope plus data is capped at
256 KiB, so mesh data has less room than the core's standalone body limit.
Attempt metadata can change; persisted data bytes, event/delivery identities and
artifact binding must remain frozen across retries. A production receiver inbox,
key distribution/rotation and KMS integration remain later work.

## Object-first acceptance

The synthetic Go acceptor verifies Stripe authenticity, then validates bounded JSON
and the chosen `customer.updated` family. It encrypts original bytes with AES-256-GCM
in a single-writer, route-scoped object directory. Content addressing, file fsync,
atomic linking and directory fsync precede the database transaction. Existing
objects are authenticated and compared rather than overwritten. Operational objects
are separate from redacted telemetry and are never expired by its 24-hour sweeper.
The development spool admits at most 64 MiB/10,000 directory entries.

PostgreSQL atomically inserts the scoped provider event and pending job, with
`synchronous_commit=on`. Uniqueness includes tenant/environment/route/account/event.
A duplicate with changed original bytes is an integrity conflict. Success follows
commit; a lost response resolves to the same event/job on retry. There is no NATS
publish requirement before this receipt and no business-delivery success claim.

Process-exit tests exercise all four requested crash boundaries. These demonstrate
local-process recovery, not power-loss durability, replicated commit, managed
object-store acknowledgements, quotas across writers, backup/restore or downstream
effectively-once behavior. Orphans remain bounded and require explicit reconciliation;
automatic GC, outbox publisher, workers and production ingress are not implemented.

References: [Envoy ext_proc](https://www.envoyproxy.io/docs/envoy/v1.36.0/configuration/http/http_filters/ext_proc_filter),
[Wasmtime embedding](https://docs.rs/wasmtime/49.0.3/wasmtime/),
[Stripe raw signature verification](https://docs.stripe.com/webhooks/signature),
[Shopify HTTPS webhooks](https://shopify.dev/docs/apps/build/webhooks/subscribe/https),
[JWS detached content](https://www.rfc-editor.org/rfc/rfc7515#appendix-F).
