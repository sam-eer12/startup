#!/usr/bin/env python3
"""Prepare explicit requests using repository-owned Stripe synthetic fixtures."""

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def main():
    output = ROOT / ".local/week3-requests"
    output.mkdir(mode=0o700, parents=True, exist_ok=True)
    contract = json.loads((ROOT / "connectors/stripe/consumer-contract.json").read_text())
    requests = {
        "import": {"contract": contract, "pointer": "", "source_label": "reviewed-stripe-fixture"},
        "bind": {
            "route_id": "route_stripe",
            "contract_id": "contract_stripe",
            "contract_version": 1,
        },
        "drift": {
            "route_id": "route_stripe",
            "body": (ROOT / "fixtures/synthetic/stripe/field-move.json").read_text(),
            "context": {
                "http_status": 200,
                "content_type": "application/json",
                "content_encoding": "identity",
                "capture_quality": "complete",
                "sample_probability": 1.0,
                "business_decline": False,
                "invariant_violation": False,
            },
        },
    }
    for name, request in requests.items():
        (output / f"{name}.json").write_text(json.dumps(request, indent=2) + "\n")
    print("Synthetic Week 3 requests written to .local/week3-requests")


if __name__ == "__main__":
    main()
