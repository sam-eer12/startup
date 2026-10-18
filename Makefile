GO ?= go
CARGO ?= cargo
PYTHON ?= .venv/bin/python
COMPOSE_PROVIDER ?= podman compose
COMPOSE = $(COMPOSE_PROVIDER) --env-file deploy/compose/images.env -f deploy/compose/compose.yaml
WEEK2_COMPOSE = $(COMPOSE) -f deploy/compose/week2.yaml

.PHONY: help bootstrap build dev test lint fmt check schemas oracle smoke up down compose-check scan
help:
	@echo "bootstrap  Install hash-locked Python tooling into .venv"
	@echo "dev        Run the native local simulator/collector on 127.0.0.1:8081"
	@echo "check      Run formatting, lint, schema, Go/Rust tests and independent oracle"
	@echo "up/down    Start/stop the Linux Compose environment (retains local volumes)"
	@echo "smoke      Exercise the native service; use SMOKE_URL=http://127.0.0.1:8080 for Envoy"
	@echo "scan       Scan dependencies against current vulnerability databases (network required)"

bootstrap:
	python3 -m venv .venv
	.venv/bin/python -m pip install --require-hashes -r requirements-dev.txt

build:
	$(GO) build -trimpath -o bin/devserver ./cmd/devserver
	$(GO) build -trimpath -o bin/meshctl ./cmd/meshctl
	$(CARGO) build --workspace --locked

dev:
	$(GO) run ./cmd/devserver

fmt:
	$(GO) fmt ./...
	$(CARGO) fmt --all
	.venv/bin/ruff format scripts

lint:
	@test -z "$$($(GO) fmt ./...)" || (echo "Go formatting changed; run make fmt"; exit 1)
	$(GO) vet ./...
	$(CARGO) fmt --all -- --check
	$(CARGO) clippy --workspace --all-targets --locked -- -D warnings
	.venv/bin/ruff check scripts
	.venv/bin/ruff format --check scripts

schemas:
	$(PYTHON) scripts/seed_fixtures.py --check
	$(PYTHON) scripts/validate_schemas.py

test: wasm
	$(GO) test -race ./...
	$(CARGO) test --workspace --locked

.PHONY: wasm week2-up week2-down week2-check
wasm:
	$(CARGO) build --release --locked -p adapter-wasm --target wasm32-unknown-unknown

week2-up:
	$(WEEK2_COMPOSE) up -d --build --wait --wait-timeout 180

week2-down:
	$(WEEK2_COMPOSE) down

week2-check:
	$(WEEK2_COMPOSE) run --rm --build durability-tests
	$(WEEK2_COMPOSE) run --rm --no-deps durability-tests python3 scripts/week2_smoke.py --url http://envoy:8080

oracle:
	$(CARGO) run --locked -p verifier

check: lint schemas test oracle

SMOKE_URL ?= http://127.0.0.1:8081
smoke:
	$(PYTHON) scripts/smoke.py --url $(SMOKE_URL)

compose-check:
	$(COMPOSE) config --quiet
	$(COMPOSE) run --rm --no-deps envoy --mode validate -c /etc/envoy/envoy.yaml

up:
	$(COMPOSE) up -d --build --wait --wait-timeout 120

down:
	$(COMPOSE) down

scan:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	$(CARGO) audit
