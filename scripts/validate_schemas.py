#!/usr/bin/env python3
"""Validate wire schemas, reviewed fixture provenance, and fail-closed examples."""

import copy
import hashlib
import json
import re
from datetime import datetime
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parents[1]
CHECKER = FormatChecker()


@CHECKER.checks("date-time")
def rfc3339_timestamp(value):
    # The library's RFC3339 checker otherwise requires an optional dependency.
    if not isinstance(value, str):
        return True
    pattern = r"\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[Zz]|[+-]\d{2}:\d{2})"
    if not re.fullmatch(pattern, value):
        return False
    try:
        return datetime.fromisoformat(value.upper()).tzinfo is not None
    except ValueError:
        return False


def load(path):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key in {path}")
            result[key] = value
        return result

    return json.loads((ROOT / path).read_text(), object_pairs_hook=unique)


SCHEMAS = {
    p.stem.removesuffix(".schema"): load(p.relative_to(ROOT))
    for p in (ROOT / "schemas/v1").glob("*.json")
}
REGISTRY = Registry().with_resources(
    (schema["$id"], Resource.from_contents(schema)) for schema in SCHEMAS.values()
)


def validator(name):
    return Draft202012Validator(SCHEMAS[name], registry=REGISTRY, format_checker=CHECKER)


def main():
    for schema in SCHEMAS.values():
        Draft202012Validator.check_schema(schema)
    for path in sorted((ROOT / "schemas/examples").glob("*.json")):
        validator(path.stem).validate(load(path.relative_to(ROOT)))
    for route_path in sorted((ROOT / "connectors").glob("*/route.json")):
        route = load(route_path.relative_to(ROOT))
        contract = load(route_path.with_name("consumer-contract.json").relative_to(ROOT))
        validator("route").validate(route)
        validator("contract").validate(contract)
        assert (route["contract_id"], route["contract_version"]) == (
            contract["contract_id"],
            contract["contract_version"],
        )
        assert route["scope"] == "synthetic_local" and contract["authority"] == "synthetic_fixture"
        Draft202012Validator.check_schema(contract["schema"])

    manifest = load("fixtures/manifests/corpus-v1.json")
    validator("fixture-manifest").validate(manifest)
    known_files = {}
    for entry in manifest["files"]:
        assert entry["path"] not in known_files, "duplicate corpus file"
        path = ROOT / entry["path"]
        assert path.resolve().is_relative_to(ROOT), "fixture escaped repository"
        assert entry["sha256"] == "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest(), (
            f"fixture hash changed: {path}"
        )
        known_files[entry["path"]] = True
    taxonomy = {case["taxonomy"] for case in manifest["cases"]}
    assert {
        "rename_move",
        "deletion",
        "nullable_missing",
        "incompatible_type",
        "enum_change",
        "unit_change",
        "auth",
        "rate_limiting",
        "pagination",
    } <= taxonomy
    case_ids = set()
    for case in manifest["cases"]:
        assert case["case_id"] not in case_ids, "duplicate case ID"
        case_ids.add(case["case_id"])
        assert case["input"] in known_files
        contract = load(f"connectors/{case['connector']}/consumer-contract.json")["schema"]
        target = Draft202012Validator(contract)
        if case["classification"] == "no_drift":
            target.validate(load(case["input"]))
        if case["classification"] == "repairable":
            assert case["expected_output"] in known_files
            target.validate(load(case["expected_output"]))
            assert not target.is_valid(load(case["input"])), (
                "move fixture no longer exercises consumer mismatch"
            )
        if case["taxonomy"] == "auth":
            assert case["http_status"] == 401 and case["classification"] == "unsupported"
        if case["taxonomy"] == "rate_limiting":
            assert case["http_status"] == 429 and case["classification"] == "unsupported"
    for family in ["stripe", "shopify", "netsuite", "generic-json"]:
        labels = {
            case["classification"] for case in manifest["cases"] if case["connector"] == family
        }
        assert {"repairable", "unsupported", "no_drift"} <= labels

    # These checks assert meaningful schema boundaries instead of mirroring every
    # field. Loading JSON must not silently grant authority or ignore operations.
    observation = load("schemas/examples/observation.json")
    for mutation in [
        {"authoritative": True},
        {"schema_version": 2},
        {"unknown_critical": True},
        {"tenant_id": "../other-tenant"},
        {"observed_at": "2026-10-18T12:00:00"},
        {"observed_at": "2026-02-30T12:00:00Z"},
    ]:
        bad = {**observation, **mutation}
        assert not validator("observation").is_valid(bad), (
            f"unsafe observation accepted: {mutation}"
        )
    adapter = load("schemas/examples/adapter-dsl.json")
    for operation in [{"op": "execute", "shell": "untrusted"}, {"op": "remove", "path": "/id"}]:
        bad = copy.deepcopy(adapter)
        bad["operations"] = [operation]
        assert not validator("adapter-dsl").is_valid(bad), "unsafe DSL vocabulary accepted"
    bad = copy.deepcopy(adapter)
    bad["operations"][0]["path"] = "/invalid~escape"
    assert not validator("adapter-dsl").is_valid(bad), "invalid JSON Pointer escape accepted"
    print(
        f"schemas: {len(SCHEMAS)} definitions, 7 wire examples, {len(manifest['cases'])} labeled synthetic cases, provenance and negative checks passed"
    )


if __name__ == "__main__":
    main()
