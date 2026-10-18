#!/usr/bin/env python3
"""Explicit maintainer action: resolve declared image tags to OCI index digests.

Normal builds consume images.env and do not resolve moving tags. Review the lock
diff and rerun the Linux smoke test when updating these versions.
"""

import json
import urllib.request
from pathlib import Path

IMAGES = {
    "GO_IMAGE": ("library/golang", "1.27.2-bookworm"),
    "RUST_IMAGE": ("library/rust", "1.99.1-bookworm"),
    "POSTGRES_IMAGE": ("library/postgres", "18.6-bookworm"),
    "NATS_IMAGE": ("library/nats", "2.12.4-alpine"),
    "ENVOY_IMAGE": ("envoyproxy/envoy", "v1.36.13"),
}


def resolve(repository, tag):
    url = f"https://auth.docker.io/token?service=registry.docker.io&scope=repository:{repository}:pull"
    with urllib.request.urlopen(url, timeout=30) as response:
        token = json.load(response)["token"]
    request = urllib.request.Request(
        f"https://registry-1.docker.io/v2/{repository}/manifests/{tag}",
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json",
        },
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        digest = response.headers["Docker-Content-Digest"]
        manifest = json.load(response)
    platforms = {
        (m.get("platform", {}).get("os"), m.get("platform", {}).get("architecture"))
        for m in manifest.get("manifests", [])
    }
    if not {("linux", "amd64"), ("linux", "arm64")} <= platforms:
        raise ValueError(f"missing supported Linux platforms: {repository}:{tag}")
    if not digest.startswith("sha256:") or len(digest) != 71:
        raise ValueError("invalid registry digest")
    return f"{repository}:{tag}@{digest}"


if __name__ == "__main__":
    lines = ["# OCI multi-platform index digests; update with python3 scripts/lock_images.py."]
    for name, (repository, tag) in IMAGES.items():
        value = resolve(repository, tag)
        print(f"resolved {repository}:{tag}")
        lines.append(f"{name}={value}")
    path = Path(__file__).resolve().parents[1] / "deploy/compose/images.env"
    path.write_text("\n".join(lines) + "\n")
