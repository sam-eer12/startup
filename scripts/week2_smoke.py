"""Exercise the synthetic Linux Envoy -> ext_proc -> Wasmtime path."""

import argparse
import concurrent.futures
import json
import urllib.error
import urllib.request

BASE_URL = "http://127.0.0.1:8080"


def fetch(path):
    try:
        with urllib.request.urlopen(BASE_URL + path, timeout=10) as reply:
            return reply.status, reply.headers, reply.read()
    except urllib.error.HTTPError as error:
        return error.code, error.headers, error.read()


def check(path):
    status, headers, body = fetch(path)
    assert status == 200, (path, status)
    value = json.loads(body)
    assert value["id"] == "evt_synthetic_001"
    metadata = value["data"]["object"]["metadata"]
    if path.startswith("/runtime/move"):
        assert metadata["display_label"] == "Local demo"
        assert "label" not in metadata["presentation"]
        assert "etag" not in headers
        assert "content-digest" not in headers
    else:
        assert metadata["presentation"]["label"] == "Local demo"
    if headers.get("Content-Length"):
        assert int(headers["Content-Length"]) == len(body)


def main():
    global BASE_URL
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default=BASE_URL)
    BASE_URL = parser.parse_args().url.rstrip("/")
    for operation in ("move", "identity"):
        check("/runtime/" + operation)
        check("/runtime/" + operation + "?case=chunked")
        assert fetch("/runtime/" + operation + "?case=empty")[0] == 204
        assert fetch("/runtime/" + operation + "?case=malformed")[0] == 422
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
        list(executor.map(check, ["/runtime/move"] * 16))
    print("Week 2 Envoy smoke passed: no-op, move, chunking, empty, malformed, concurrency")


if __name__ == "__main__":
    main()
