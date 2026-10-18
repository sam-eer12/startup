#!/usr/bin/env python3
"""Rebuild the explicitly authored synthetic corpus; never import customer data.

Golden outputs and consumer schemas are defined here independently of any
synthesizer. Changing this file changes the oracle and requires review.
"""

import argparse
import copy
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DIALECT = "https://json-schema.org/draft/2020-12/schema"


def object_schema(properties, required):
    return {
        "type": "object",
        "properties": properties,
        "required": required,
        "additionalProperties": True,
    }


STRING = {"type": "string"}
FAMILIES = {
    "stripe": {
        "event": "customer.updated",
        "direction": "webhook",
        "from": "/data/object/metadata/presentation/label",
        "to": "/data/object/metadata/display_label",
        "protected": ["/id", "/type", "/data/object/id"],
        "baseline": {
            "id": "evt_synthetic_001",
            "type": "customer.updated",
            "api_version": "fixture-v1",
            "data": {
                "object": {
                    "id": "cus_synthetic_001",
                    "object": "customer",
                    "email": "synthetic@example.invalid",
                    "metadata": {"display_label": "Local demo"},
                }
            },
        },
        "moved": {
            "id": "evt_synthetic_001",
            "type": "customer.updated",
            "api_version": "fixture-v1",
            "data": {
                "object": {
                    "id": "cus_synthetic_001",
                    "object": "customer",
                    "email": "synthetic@example.invalid",
                    "metadata": {"presentation": {"label": "Local demo"}},
                }
            },
        },
        "golden": {
            "id": "evt_synthetic_001",
            "type": "customer.updated",
            "api_version": "fixture-v1",
            "data": {
                "object": {
                    "id": "cus_synthetic_001",
                    "object": "customer",
                    "email": "synthetic@example.invalid",
                    "metadata": {"presentation": {}, "display_label": "Local demo"},
                }
            },
        },
        "schema": object_schema(
            {
                "id": STRING,
                "type": {"const": "customer.updated"},
                "data": object_schema(
                    {
                        "object": object_schema(
                            {
                                "id": STRING,
                                "metadata": object_schema(
                                    {"display_label": STRING}, ["display_label"]
                                ),
                            },
                            ["id", "metadata"],
                        )
                    },
                    ["object"],
                ),
            },
            ["id", "type", "data"],
        ),
        "delete_parent": ["data", "object"],
        "delete_key": "id",
    },
    "shopify": {
        "event": "products/update",
        "direction": "webhook",
        "from": "/details/vendor",
        "to": "/vendor",
        "protected": ["/id", "/status"],
        "baseline": {
            "id": 9007199254740993,
            "title": "Synthetic product",
            "vendor": "Local vendor",
            "status": "active",
        },
        "moved": {
            "id": 9007199254740993,
            "title": "Synthetic product",
            "details": {"vendor": "Local vendor"},
            "status": "active",
        },
        "golden": {
            "id": 9007199254740993,
            "title": "Synthetic product",
            "details": {},
            "vendor": "Local vendor",
            "status": "active",
        },
        "schema": object_schema(
            {
                "id": {"type": "integer"},
                "vendor": STRING,
                "status": {"enum": ["active", "draft", "archived"]},
            },
            ["id", "vendor", "status"],
        ),
        "delete_parent": [],
        "delete_key": "id",
    },
    "netsuite": {
        "event": "GET inventoryItem/{id}",
        "direction": "response",
        "from": "/presentation/displayName",
        "to": "/displayName",
        "protected": ["/id", "/itemId"],
        "baseline": {
            "id": "fixture-item-001",
            "itemId": "SKU-LOCAL-001",
            "displayName": "Synthetic item",
            "isInactive": False,
        },
        "moved": {
            "id": "fixture-item-001",
            "itemId": "SKU-LOCAL-001",
            "presentation": {"displayName": "Synthetic item"},
            "isInactive": False,
        },
        "golden": {
            "id": "fixture-item-001",
            "itemId": "SKU-LOCAL-001",
            "presentation": {},
            "displayName": "Synthetic item",
            "isInactive": False,
        },
        "schema": object_schema(
            {"id": STRING, "itemId": STRING, "displayName": STRING}, ["id", "itemId", "displayName"]
        ),
        "delete_parent": [],
        "delete_key": "id",
    },
    "generic-json": {
        "event": "shipment.updated",
        "direction": "webhook",
        "from": "/shipment/tracking/number",
        "to": "/shipment/tracking_number",
        "protected": ["/id", "/shipment/id", "/shipment/status", "/shipment/weight_grams"],
        "baseline": {
            "id": "event-local-001",
            "shipment": {
                "id": "shipment-local-001",
                "tracking_number": "TRACK-0001",
                "status": "in_transit",
                "weight_grams": 1000,
                "note": None,
            },
        },
        "moved": {
            "id": "event-local-001",
            "shipment": {
                "id": "shipment-local-001",
                "tracking": {"number": "TRACK-0001"},
                "status": "in_transit",
                "weight_grams": 1000,
                "note": None,
            },
        },
        "golden": {
            "id": "event-local-001",
            "shipment": {
                "id": "shipment-local-001",
                "tracking": {},
                "tracking_number": "TRACK-0001",
                "status": "in_transit",
                "weight_grams": 1000,
                "note": None,
            },
        },
        "schema": object_schema(
            {
                "id": STRING,
                "shipment": object_schema(
                    {
                        "id": STRING,
                        "tracking_number": STRING,
                        "status": {"enum": ["in_transit", "delivered"]},
                        "weight_grams": {"type": "integer"},
                    },
                    ["id", "tracking_number", "status", "weight_grams"],
                ),
            },
            ["id", "shipment"],
        ),
        "delete_parent": ["shipment"],
        "delete_key": "id",
    },
}


def encode(value):
    return (json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n").encode()


def generate():
    outputs = {}
    cases = []

    def save(path, value):
        outputs[path] = encode(value)
        return path

    def case(
        family,
        scenario,
        value,
        taxonomy,
        classification,
        reason,
        output=None,
        assertions=None,
        status=200,
    ):
        path = save(f"fixtures/synthetic/{family}/{scenario}.json", value)
        cases.append(
            {
                "case_id": f"case_{family}_{scenario}",
                "connector": family,
                "taxonomy": taxonomy,
                "classification": classification,
                "reason": reason,
                "input": path,
                "http_status": status,
                "expected_output": output,
                "assertions": assertions or [],
            }
        )

    for family, data in FAMILIES.items():
        contract = {
            "schema_version": 1,
            "tenant_id": "tenant_local",
            "environment_id": "env_local",
            "contract_id": f"contract_{family}",
            "contract_version": 1,
            "authority": "synthetic_fixture",
            "owner": "synthetic-fixture-author",
            "dialect": DIALECT,
            "schema": {"$schema": DIALECT, **data["schema"]},
        }
        save(f"connectors/{family}/consumer-contract.json", contract)
        save(
            f"connectors/{family}/route.json",
            {
                "schema_version": 1,
                "tenant_id": "tenant_local",
                "environment_id": "env_local",
                "route_id": f"route_{family}",
                "connector": family,
                "provider_version": "fixture-v1",
                "event_family": data["event"],
                "direction": data["direction"],
                "mode": "observe",
                "scope": "synthetic_local",
                "owner": "synthetic-fixture-author",
                "contract_id": f"contract_{family}",
                "contract_version": 1,
                "payload_policy": {
                    "locality": "local_only",
                    "telemetry_retention_hours": 24,
                    "durable_acceptance": False,
                    "signature_trust": "unverified_fixture",
                    "redaction_policy": "all-scalars-and-keys-v1",
                },
            },
        )
        golden = save(f"fixtures/golden/{family}/field-move.json", data["golden"])
        assertions = [
            {"kind": "value_equal", "before": data["from"], "after": data["to"]},
            *[{"kind": "value_equal", "before": path, "after": path} for path in data["protected"]],
            {"kind": "unchanged_outside", "paths": [data["from"], data["to"]]},
            {"kind": "absent", "path": data["from"]},
        ]
        case(
            family,
            "baseline",
            data["baseline"],
            "baseline",
            "no_drift",
            "Independently authored synthetic consumer-compatible payload.",
        )
        case(
            family,
            "field-move",
            data["moved"],
            "rename_move",
            "repairable",
            "Explicit synthetic equivalence for a noncritical display/tracking field; no destination collision.",
            golden,
            assertions,
        )
        missing = copy.deepcopy(data["baseline"])
        parent = missing
        for key in data["delete_parent"]:
            parent = parent[key]
        del parent[data["delete_key"]]
        case(
            family,
            "required-deletion",
            missing,
            "deletion",
            "unsupported",
            "Required identity information is missing; never manufacture it.",
        )

    family = "generic-json"
    baseline = FAMILIES[family]["baseline"]
    for scenario, field, value, taxonomy, reason in [
        (
            "nullable",
            "tracking_number",
            None,
            "nullable_missing",
            "Null is not a missing or valid string tracking identifier.",
        ),
        (
            "incompatible-type",
            "tracking_number",
            123,
            "incompatible_type",
            "Opaque tracking identifiers must never be coerced to numbers.",
        ),
        (
            "enum-change",
            "status",
            "returned_to_sender",
            "enum_change",
            "Unknown business state needs an owner decision.",
        ),
        (
            "unit-change",
            "weight_grams",
            1,
            "unit_change",
            "Schema-valid but wrong unit: shape validation cannot prove business meaning.",
        ),
    ]:
        value_case = copy.deepcopy(baseline)
        value_case["shipment"][field] = value
        case(family, scenario, value_case, taxonomy, "unsupported", reason)
    missing = copy.deepcopy(baseline)
    del missing["shipment"]["tracking_number"]
    case(
        family,
        "missing",
        missing,
        "nullable_missing",
        "unsupported",
        "Missing required tracking identifier has no equivalent source.",
    )
    collision = copy.deepcopy(FAMILIES[family]["moved"])
    collision["shipment"]["tracking_number"] = "CONFLICTING-VALUE"
    case(
        family,
        "collision",
        collision,
        "rename_move",
        "unsupported",
        "An existing destination blocks a move even if the source looks plausible.",
    )
    case(
        family,
        "auth",
        {"error": "invalid_credentials"},
        "auth",
        "unsupported",
        "401 is an operational/security failure, not schema drift.",
        status=401,
    )
    case(
        family,
        "rate-limiting",
        {"error": "rate_limited"},
        "rate_limiting",
        "unsupported",
        "429 needs bounded operational backoff, not a payload adapter.",
        status=429,
    )
    case(
        family,
        "pagination",
        {"items": [], "next": "same-cursor", "previous": "same-cursor"},
        "pagination",
        "unsupported",
        "A repeated pagination cursor requires sequence evidence; no single-body repair.",
    )

    manifest = {
        "schema_version": 1,
        "corpus_id": "corpus_synthetic_v1",
        "origin": "synthetic",
        "author": "synthetic-fixture-author",
        "created_at": "2026-10-18T00:00:00Z",
        "provenance": "Hand-authored local examples and invented mutations; no provider incident, account capture or partner approval is claimed.",
        "license": "Repository-owned synthetic test data; no third-party payloads.",
        "contains_customer_data": False,
        "eligible_for_release": False,
        "holdout_cases": 0,
        "cases": cases,
        "files": [
            {"path": path, "sha256": "sha256:" + hashlib.sha256(body).hexdigest()}
            for path, body in sorted(outputs.items())
        ],
    }
    save("fixtures/manifests/corpus-v1.json", manifest)
    return outputs


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="check without overwriting fixtures")
    args = parser.parse_args()
    for path, body in generate().items():
        target = ROOT / path
        if args.check:
            if not target.exists() or target.read_bytes() != body:
                raise SystemExit(f"fixture differs from reviewed seed: {path}")
        else:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(body)
    print("synthetic fixture seed checked" if args.check else "synthetic fixture seed written")


if __name__ == "__main__":
    main()
