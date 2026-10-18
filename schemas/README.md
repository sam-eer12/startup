# Versioned wire contracts

`v1/*.schema.json` uses JSON Schema 2020-12. `common` reserves stable identifiers,
SHA-256 digests, JSON Pointers and RFC3339 timestamps. The seven primary wire
contracts are route, consumer contract, observation, adapter DSL, verification
manifest, release manifest and internal event. The fixture manifest has a separate
schema and provenance rules.

Examples are **unsigned documentation**, with placeholder hashes and identities.
They cannot authorize builds, repairs or releases. JSON Schema validation checks
wire shape; future services must additionally check ownership, signatures, approval
freshness, monotonic generations, hash consistency and policy eligibility.

`schema_version: 1` is required and unknown critical fields are rejected. Changes
must be additive where old readers remain safe; otherwise introduce a new version.
The Rust operation enum mirrors the vocabulary only: deserialization is not
authorization or executable-plan validation. The full typed IR and compiler follow
in Weeks 3–4. Agent gRPC and the Wasm ABI are not implemented in Week 1.

All local observations require `authoritative: false`. Consumer contracts originate
from explicit synthetic fixtures or a future owner-approved import, never inferred
automatically from observations. Fixture routes require observe mode, disabled
durable acceptance and an unverified signature trust state.

Run `make schemas` to validate the definitions, examples, fixture hashes, target
schemas and negative cases. Reference resolution is from the repository's registry;
validation does not fetch schema references from the network.
