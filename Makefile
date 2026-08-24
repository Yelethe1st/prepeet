# Prepeet task runner.
#
# One entry point for the three deployables, so a task is spelled the same way
# on a laptop and in CI. PLT-01 requires a new engineer to start the whole stack
# from a single documented command; PLT-10 requires the test and coverage gates
# to be the easy path rather than the diligent one.
#
# Run `make help` for the list.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

GO_DIR  := services/platform
PY_DIR  := services/intelligence
WEB_DIR := apps/web

# Coverage floors. These are gates against erosion, not targets: meeting them
# proves nothing about whether the invariants are tested. Raise them as suites
# mature, and never lower one to make a build pass.
GO_COVERAGE_MIN  := 80
PY_COVERAGE_MIN  := 80
WEB_COVERAGE_MIN := 80

# cmd packages are process wiring: a main that reads configuration, builds
# dependencies and starts a server. They are deliberately kept thin and are
# covered by the smoke tests in CI rather than by unit tests, so they are
# excluded from the unit coverage figure rather than inflating it.
GO_COVER_PKGS := ./platform/... ./internal/...

.PHONY: help
help: ## List the available tasks
	@grep -hE '^[a-z][a-zA-Z0-9_-]*:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ----------------------------------------------------------------- bootstrap

.PHONY: bootstrap
bootstrap: bootstrap-go bootstrap-py bootstrap-web ## Install every toolchain and dependency

.PHONY: bootstrap-go
bootstrap-go:
	cd $(GO_DIR) && go mod download

.PHONY: bootstrap-py
bootstrap-py:
	cd $(PY_DIR) && uv sync --all-extras

.PHONY: bootstrap-web
bootstrap-web:
	cd $(WEB_DIR) && pnpm install --frozen-lockfile

# ----------------------------------------------------------------------- test

.PHONY: test
test: test-go test-py test-web ## Run every test suite

.PHONY: test-go
test-go: ## Run the Go suite with the race detector
	cd $(GO_DIR) && go test -race ./...

.PHONY: test-py
test-py: ## Run the Python suite
	cd $(PY_DIR) && uv run pytest

.PHONY: test-web
test-web: ## Run the web suite
	cd $(WEB_DIR) && pnpm test

.PHONY: test-integration
test-integration: ## Run the integration suites against real dependencies in containers
	cd $(GO_DIR) && go test -tags integration -timeout 15m ./...

.PHONY: watch-go
watch-go: ## Re-run the Go suite on change, for the red-green loop
	cd $(GO_DIR) && go test ./... -count=1 -failfast

# ------------------------------------------------------------------- coverage

.PHONY: cover
cover: cover-go cover-py cover-web ## Run every suite and enforce the coverage floors

.PHONY: cover-go
# Coverage runs with the integration tag so the storage adapter is measured by
# the tests that actually exercise it. Without the tag the adapter reads as
# untested, which is both wrong and the kind of number that invites lowering the
# floor. This needs Docker; `make test-go` stays fast for the red-green loop.
cover-go:
	cd $(GO_DIR) && go test -tags integration -race -timeout 15m -coverprofile=coverage.out -covermode=atomic $(GO_COVER_PKGS)
	@cd $(GO_DIR) && total=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}'; ) \
		&& ../../tools/coverage/check.sh go "$$total" $(GO_COVERAGE_MIN)

.PHONY: cover-py
cover-py:
	cd $(PY_DIR) && uv run pytest --cov=prepeet_ai --cov-report=term-missing --cov-fail-under=$(PY_COVERAGE_MIN)

.PHONY: cover-web
cover-web:
	cd $(WEB_DIR) && pnpm test:coverage

# ------------------------------------------------------------------ contracts

TOOLS_BIN := $(CURDIR)/.tools

.PHONY: tools
tools: ## Install the pinned code generators into .tools
	@mkdir -p $(TOOLS_BIN)
	cd $(GO_DIR) && GOBIN=$(TOOLS_BIN) go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

.PHONY: generate
generate: tools ## Regenerate every client and server type from the contracts
	# Generation runs from the repository root, because the generator resolves
	# output paths against the working directory rather than against its config.
	$(TOOLS_BIN)/oapi-codegen -config packages/contracts/api/oapi-codegen.yaml packages/contracts/api/openapi.yaml
	pnpm generate:api
	cd packages/generated/go && go mod tidy

.PHONY: check-generated
check-generated: generate ## Fail if generated code differs from the contracts
	@if [ -n "$$(git status --porcelain packages/generated)" ]; then \
		echo "Generated code is out of date. Run make generate and commit the result:"; \
		git --no-pager diff --stat packages/generated; \
		exit 1; \
	fi
	@printf '\033[32mPASS\033[0m generated code matches the contracts\n'

.PHONY: lint-contracts
lint-contracts: ## Lint the OpenAPI document
	pnpm lint:contracts

# ----------------------------------------------------------------- lint / fmt

.PHONY: lint
lint: lint-contracts lint-go lint-py lint-web ## Lint and type check everything

.PHONY: lint-go
lint-go:
	cd $(GO_DIR) && gofmt -l . | tee /dev/stderr | (! read -r) && go vet ./...

.PHONY: lint-py
lint-py:
	cd $(PY_DIR) && uv run ruff check . && uv run ruff format --check . && uv run mypy src

.PHONY: lint-web
lint-web:
	cd $(WEB_DIR) && pnpm lint && pnpm typecheck

.PHONY: fmt
fmt: ## Format everything in place
	cd $(GO_DIR) && gofmt -w .
	cd $(PY_DIR) && uv run ruff format .
	cd $(WEB_DIR) && pnpm format

# --------------------------------------------------------------- local stack

COMPOSE := docker compose -f infrastructure/local/docker-compose.yml

.PHONY: local-up
local-up: ## Start PostgreSQL, LocalStack and Temporal locally
	$(COMPOSE) up -d --wait
	@echo "  postgres      localhost:5432"
	@echo "  localstack    localhost:4566   (s3, secretsmanager, kms)"
	@echo "  temporal      localhost:7233   ui localhost:8233   namespace prepeet-local"

.PHONY: local-down
local-down: ## Stop the local stack, keeping its data
	$(COMPOSE) down

.PHONY: local-reset
local-reset: ## Stop the local stack and delete its data
	$(COMPOSE) down --volumes

.PHONY: local-logs
local-logs: ## Follow the local stack logs
	$(COMPOSE) logs -f

# ------------------------------------------------------------------ run local

.PHONY: dev
dev: ## Print how to start each deployable locally
	@echo "  make dev-api    the Go API on :8080"
	@echo "  make dev-worker the Temporal worker"
	@echo "  make dev-web    the Next.js application on :3000"

.PHONY: dev-api
dev-api: ## Run the Go API
	cd $(GO_DIR) && go run ./cmd/api

.PHONY: dev-worker
dev-worker: ## Run the Temporal worker
	cd $(GO_DIR) && go run ./cmd/worker

.PHONY: dev-web
dev-web: ## Run the Next.js application
	cd $(WEB_DIR) && pnpm dev

.PHONY: migrate
migrate: ## Apply database migrations
	cd $(GO_DIR) && go run ./cmd/migrate

# ------------------------------------------------------------------------- ci

.PHONY: ci
ci: lint check-generated cover ## Everything CI runs, in the order CI runs it
