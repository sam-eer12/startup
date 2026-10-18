#!/usr/bin/env python3
"""Exercise real local HTTP fixture -> collector traffic, optionally via Envoy."""

import argparse
import json
import time
import urllib.error
import urllib.request

from validate_schemas import validator


def request(base, path, body=None, headers=None):
    req = urllib.request.Request(base + path, data=body, headers=headers or {})
    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:8081")
    args = parser.parse_args()
    for _ in range(30):
        try:
            if request(args.url, "/healthz")[0] == 200:
                break
        except OSError, urllib.error.URLError:
            pass
        time.sleep(1)
    else:
        raise SystemExit("local service did not become healthy")

    for family in ["stripe", "shopify", "netsuite", "generic-json"]:
        for scenario in ["baseline", "field-move", "required-deletion"]:
            status, body = request(args.url, f"/fixtures/{family}/{scenario}")
            assert status == 200
            status, observation = request(
                args.url,
                f"/observe/route_{family}",
                body,
                {
                    "Content-Type": "application/json",
                    "Authorization": "Bearer SEEDED-SECRET",
                    "X-Mesh-Tenant-Id": "tenant_attacker",
                },
            )
            assert status == 200, observation
            result = json.loads(observation)
            validator("observation").validate(result)
            assert result["tenant_id"] == "tenant_local" and not result["authoritative"]
            assert result["signature_status"] == "unverified_fixture"
            assert b"SEEDED-SECRET" not in observation
    for scenario, expected in [("auth", 401), ("rate-limiting", 429)]:
        assert request(args.url, f"/fixtures/generic-json/{scenario}")[0] == expected
    status, _ = request(
        args.url,
        "/observe/route_stripe",
        b'{"duplicate":1,"duplicate":2}',
        {"Content-Type": "application/json"},
    )
    assert status == 422
    status, metrics = request(args.url, "/metrics")
    assert status == 200 and b'mesh_capture_samples_total{outcome="captured"}' in metrics
    print(
        "HTTP smoke passed: four fixture families, redacted observation schema, tenant-spoof rejection, operational statuses and metrics"
    )


if __name__ == "__main__":
    main()
