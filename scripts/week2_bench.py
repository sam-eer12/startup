"""Diagnostic Envoy latency sweep; an undersized VM is not M0 qualification."""

import argparse
import concurrent.futures
import json
import math
import platform
import time
import urllib.error
import urllib.request


def request(url, mode):
    started = time.perf_counter_ns()
    try:
        with urllib.request.urlopen(url, timeout=10) as response:
            raw = response.read()
            value = json.loads(raw)
            assert value["id"] == "evt_synthetic_001"
            metadata = value["data"]["object"]["metadata"]
            if mode == "move":
                assert metadata["display_label"] == "Local demo"
                assert "label" not in metadata["presentation"]
            else:
                assert metadata["presentation"]["label"] == "Local demo"
        return (time.perf_counter_ns() - started) / 1e6, None
    except urllib.error.HTTPError as error:
        return (time.perf_counter_ns() - started) / 1e6, f"HTTP_{error.code}"
    except Exception as error:
        return (time.perf_counter_ns() - started) / 1e6, type(error).__name__


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://envoy:8080")
    parser.add_argument("--requests", type=int, default=100)
    args = parser.parse_args()
    assert args.requests >= 10
    results = []
    for size in (32768, 262144):
        for concurrency in (1, 4):
            for mode in ("bypass", "identity", "move"):
                url = f"{args.url}/runtime/{mode}?size={size}"
                warmup = [request(url, mode) for _ in range(10)]
                started = time.perf_counter()
                with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
                    samples = list(pool.map(lambda _: request(url, mode), range(args.requests)))
                elapsed = time.perf_counter() - started
                timings = sorted(sample[0] for sample in samples)
                errors = [error for _, error in samples if error]
                results.append(
                    {
                        "body_bytes": size,
                        "concurrency": concurrency,
                        "mode": mode,
                        "requests": args.requests,
                        "failures": len(errors),
                        "error_types": sorted(set(errors)),
                        "warmup_failures": sum(bool(error) for _, error in warmup),
                        "requests_per_second": args.requests / elapsed,
                        **{
                            f"p{int(p * 100)}_ms": timings[math.ceil(p * (len(timings) - 1))]
                            for p in (0.5, 0.95, 0.99)
                        },
                    }
                )
    print(
        json.dumps(
            {
                "kind": "diagnostic_gateway_sweep_not_qualification",
                "client_platform": platform.platform(),
                "payload_distribution": "fixed 32/256 KiB; synthetic string padding",
                "transport": "HTTP/1.1 fresh client connection; ext_proc uses HTTP/2",
                "warmup_requests_per_cell": 10,
                "results": results,
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
