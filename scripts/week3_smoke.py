#!/usr/bin/env python3
"""Exercise the real local Week 3 HTTP service, CLI and restart persistence."""

import json
import signal
import socket
import subprocess
import tempfile
import time
from pathlib import Path
from urllib.error import URLError
from urllib.request import urlopen

ROOT = Path(__file__).resolve().parents[1]


def main():
    with tempfile.TemporaryDirectory(prefix="mesh-week3-") as temp:
        directory = Path(temp)
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            address = f"127.0.0.1:{reservation.getsockname()[1]}"
        base = "http://" + address
        command = [
            str(ROOT / "bin/control-lab"),
            "-listen",
            address,
            "-registry",
            str(directory / "registry"),
            "-engine",
            str(ROOT / "target/debug/contract-ir"),
        ]

        def start():
            process = subprocess.Popen(
                command, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
            )
            for _ in range(60):
                if process.poll() is not None:
                    raise RuntimeError("control lab exited before readiness")
                try:
                    with urlopen(base + "/healthz", timeout=0.25) as response:
                        assert response.status == 200
                        return process
                except URLError, TimeoutError:
                    time.sleep(0.1)
            process.terminate()
            process.wait(timeout=5)
            raise RuntimeError("control lab did not become ready")

        def stop(process):
            process.send_signal(signal.SIGTERM)
            process.wait(timeout=5)

        def cli(*args):
            result = subprocess.run(
                [str(ROOT / "bin/meshctl"), "-url", base, *args],
                capture_output=True,
                text=True,
                timeout=5,
                check=True,
            )
            return json.loads(result.stdout)

        def request_file(name, value):
            path = directory / (name + ".json")
            path.write_text(json.dumps(value))
            return str(path)

        contract = json.loads((ROOT / "connectors/stripe/consumer-contract.json").read_text())
        import_path = request_file(
            "import", {"contract": contract, "pointer": "", "source_label": "smoke-fixture"}
        )
        bind_path = request_file(
            "bind",
            {"route_id": "route_stripe", "contract_id": "contract_stripe", "contract_version": 1},
        )
        drift_path = request_file(
            "drift",
            {
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
        )
        server = start()
        try:
            assert cli("import", import_path)["normalized_hash"].startswith("sha256:")
            assert cli("bind", bind_path)["contract_version"] == 1
            for index in range(3):
                result = cli("drift", drift_path)
                assert result["incident"]["state"] == ("confirmed" if index == 2 else "provisional")
                assert not result["detection"]["automatic_repair_eligible"]
            incident_id = result["incident"]["id"]
            review_path = request_file(
                "review",
                {
                    "from": "/data/object/metadata/presentation/label",
                    "to": "/data/object/metadata/display_label",
                    "reviewer": "smoke-reviewer",
                },
            )
            assert not cli("review", incident_id, review_path)["mapping_review"][
                "authorizes_repair"
            ]
            assert len(cli("incidents")["items"]) == 1
        finally:
            stop(server)
        server = start()
        try:
            restored = cli("incident", incident_id)
            assert restored["state"] == "confirmed" and restored["observations"] == 3
            assert restored["mapping_review"]["reviewer"] == "smoke-reviewer"
        finally:
            stop(server)
    print("Week 3 CLI/HTTP smoke: import, binding, drift, review and restart passed")


if __name__ == "__main__":
    main()
