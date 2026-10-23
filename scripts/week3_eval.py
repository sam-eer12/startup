#!/usr/bin/env python3
"""Evaluate the offline Rust detector against independent synthetic labels."""

import argparse
import hashlib
import json
import platform
import subprocess
from collections import Counter
from datetime import UTC, datetime
from decimal import Decimal
from pathlib import Path

from jsonschema import Draft202012Validator, validators

ROOT = Path(__file__).resolve().parents[1]
EXACT_VALIDATOR = validators.extend(
    Draft202012Validator,
    type_checker=Draft202012Validator.TYPE_CHECKER.redefine(
        "integer",
        lambda _checker, value: (
            (isinstance(value, int) and not isinstance(value, bool))
            or (isinstance(value, Decimal) and value == value.to_integral_value())
        ),
    ),
)
CONTEXT = {
    "http_status": 200,
    "content_type": "application/json",
    "content_encoding": "identity",
    "capture_quality": "complete",
    "sample_probability": 1.0,
    "business_decline": False,
    "invariant_violation": False,
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--engine", type=Path, default=ROOT / "target/debug/contract-ir")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    source = (ROOT / "fixtures/detector/cases-v1.json").read_bytes()
    corpus = json.loads(source)
    cases = corpus["cases"]
    assert len(cases) >= 50 and len({case["id"] for case in cases}) == len(cases)
    results = []
    for case in cases:
        context = CONTEXT | case.get("context", {})
        # A separate standards validator checks the structural labels. No
        # detector result is used to author or update the expected labels.
        if case["class"] in {"healthy", "structural"}:
            value = json.loads(case["body"], parse_float=Decimal)
            validator = EXACT_VALIDATOR(case["schema"])
            Draft202012Validator.check_schema(case["schema"])
            assert validator.is_valid(value) == (case["class"] == "healthy"), case["id"]
        request = {
            "operation": "detect",
            "document": case["schema"],
            "pointer": "",
            "body": case["body"],
            "context": context,
        }
        process = subprocess.run(
            [str(args.engine)],
            input=json.dumps(request),
            capture_output=True,
            text=True,
            timeout=5,
            check=True,
        )
        envelope = json.loads(process.stdout)
        assert envelope["ok"], (case["id"], envelope)
        result = envelope["result"]["detection"]
        assert result["class"] == case["class"], (case["id"], result)
        assert not result["automatic_repair_eligible"], case["id"]
        assert sorted({c["code"] for c in result["changes"]}) == sorted(case.get("codes", [])), (
            case["id"],
            result,
        )
        if "reason" in case:
            assert result["reason"] == case["reason"], (case["id"], result)
        assert result["sample_probability"] == context["sample_probability"]
        results.append({"id": case["id"], "expected": case["class"], "actual": result["class"]})
    report = {
        "schema_version": 1,
        "profile": "synthetic-offline-week3",
        "recorded_at": datetime.now(UTC).isoformat(),
        "platform": {"system": platform.system(), "architecture": platform.machine()},
        "corpus_sha256": hashlib.sha256(source).hexdigest(),
        "engine_sha256": hashlib.sha256(args.engine.read_bytes()).hexdigest(),
        "scenarios": len(cases),
        "passed": len(results),
        "classes": dict(sorted(Counter(case["class"] for case in cases).items())),
        "eligible_for_release": False,
        "semantic_limit": "Schema-valid unit/business errors require explicit invariant feedback.",
        "results": results,
    }
    if args.output:
        args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(f"Week 3 detector: {len(results)}/{len(cases)} labeled synthetic scenarios passed")


if __name__ == "__main__":
    main()
