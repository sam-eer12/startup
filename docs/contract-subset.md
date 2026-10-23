# Contract import subset v1

The local Week 3 tool accepts JSON Schema 2020-12 or an explicitly selected schema
from OpenAPI 3.1.0/3.1.1 and 3.0.0–3.0.4 **JSON documents**. This is a bounded
subset, not full dialect conformance. It performs no network or filesystem
reference fetching. Unsupported keywords block import, including unknown extensions.

| Keyword / construct | Handling |
|---|---|
| `type` | Seven JSON types; `integer` uses mathematical integrality and exact decimal normalization. Type arrays supported outside OpenAPI 3.0 |
| `properties`, `required` | Stable ordered property map and required-name set; missing and null remain distinct |
| `additionalProperties` | Boolean or supported schema; omitted means true. Unknown keys are never exported in reports |
| `items` | Single schema or boolean; heterogeneous observations retain distinct item shapes; empty arrays have no inferred item type |
| `anyOf`, `oneOf` | Up to eight branches; preserve all successful branch indexes; `oneOf` requires exactly one. Ambiguity never authorizes a mapping |
| `enum`, `const` | Exact JSON equality, including null, objects/arrays and normalized numbers; enum capped at 128 distinct members |
| `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum` | Numeric 2020-12 form only, exact decimal comparison; tokens ≤256 bytes and exponent magnitude ≤256. OpenAPI 3.0 exclusive bounds are blocked |
| `minLength`, `maxLength` | Unicode scalar count, not UTF-8 bytes; nonnegative u64 bounds |
| `minItems`, `maxItems`, `minProperties`, `maxProperties` | Nonnegative u64 bounds |
| `$ref` | Local `#` JSON Pointers only; valid `~0`/`~1` escapes; reject cycles, missing targets, percent-encoded fragments, anchors and assertion siblings |
| `$defs` | Bounded validation and resolution; unused definitions are checked when present in the selected schema |
| `$schema` | Only `https://json-schema.org/draft/2020-12/schema`; no nested dialect switch |
| `nullable` | OpenAPI 3.0 only, requires explicit `type`; true becomes a null/type union with a visible diagnostic. Ambiguous enum/const/union compositions rejected |
| `title`, `description`, `$comment` | String annotations, excluded from the IR hash |
| `default`, `examples`, `example` | Never assertions or inferred values; excluded with an annotation diagnostic |
| Boolean schemas | Supported outside OpenAPI 3.0; nested false schemas forbid the value |
| `pattern`, `patternProperties`, `format` | Blocked; no unbounded regex execution or silently ignored format assertions |
| `allOf`, `not`, conditionals, dependencies, `contains`, tuple items, `uniqueItems`, `multipleOf`, unevaluated keywords | Blocked pending explicit IR semantics and fixtures |
| `readOnly`, `writeOnly`, discriminator, semantic extensions | Blocked; route direction is explicit, and no assertion is silently stripped |
| `$id`, anchors, dynamic refs, external refs, custom vocabularies | Blocked; no alternate base URI or fetch path |

The normalized hash includes IR version and its root, with stable property order,
sorted type/required/enum sets and sorted union branches. Exact source bytes and
import metadata have separate provenance. This is structural canonicalization,
not a solver for equivalence of arbitrary schemas. The registry preserves source
differences and will not overwrite an existing version with a changed import.

Semantics were checked against the primary specifications:
[JSON Schema Core](https://json-schema.org/draft/2020-12/json-schema-core),
[JSON Schema Validation](https://json-schema.org/draft/2020-12/json-schema-validation),
and [OpenAPI 3.0.4 Schema Object](https://spec.openapis.org/oas/v3.0.4.html#schema-object).
Resource limits and local-only boundaries are in [Week 3 status](week-3.md).
