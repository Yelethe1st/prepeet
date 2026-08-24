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
	@$(COMPOSE) up -d --wait || { \
		echo ""; \
		echo "  The stack did not start. Docker's reason is above; what follows is"; \
		echo "  context, not a diagnosis."; \
		echo ""; \
		$(MAKE) --no-print-directory local-ports; \
		echo ""; \
		echo "  If a port is taken by something that is not this stack, move ours:"; \
		echo "  cp infrastructure/local/.env.example infrastructure/local/.env and change"; \
		echo "  it. The container side never changes, so nothing inside the stack cares."; \
		echo ""; \
		echo "  If the ports are fine, a service failed its healthcheck. Find which with"; \
		echo "  $(COMPOSE) ps, then read it with make local-logs."; \
		echo ""; \
		exit 1; \
	}
	@echo
	@echo "  ready:"
	@# Ports are read back from Docker rather than printed from constants. The
	@# compose file makes every host port overridable through .env, so a printed
	@# constant is wrong for exactly the developer who needed it to be right:
	@# the one who moved a port because something else already held it.
	@printf '    %-12s localhost:%s\n' postgres   "$$($(COMPOSE) port postgres 5432 | cut -d: -f2)"
	@printf '    %-12s localhost:%s   (s3, secretsmanager, kms)\n' localstack "$$($(COMPOSE) port localstack 4566 | cut -d: -f2)"
	@printf '    %-12s localhost:%s   namespace prepeet-local\n' temporal   "$$($(COMPOSE) port temporal 7233 | cut -d: -f2)"
	@printf '    %-12s localhost:%s\n' "temporal ui" "$$($(COMPOSE) port temporal-ui 8080 | cut -d: -f2)"
	@echo
	@echo "  Temporal has its own PostgreSQL, per ADR-0007. It is not published on a"
	@echo "  host port, because nothing outside the stack should be reaching it."

.PHONY: local-down
local-down: ## Stop the local stack, keeping its data
	$(COMPOSE) down

.PHONY: local-reset
local-reset: ## Stop the local stack and delete its data
	$(COMPOSE) down --volumes

.PHONY: local-logs
local-logs: ## Follow the local stack logs
	$(COMPOSE) logs -f

.PHONY: local-ports
local-ports: ## Show which local stack host ports are free and which are taken
	@# Whether a port is taken is decided by trying to connect to it, not by
	@# asking lsof. lsof without elevated privileges silently omits processes
	@# owned by other users, so a check built on it reports "free" for ports
	@# that are not, which is worse than no check: it sends you looking
	@# somewhere else. lsof is still used, but only to put a name to a port
	@# that has already been shown to be busy.
	@set -a; [ -f infrastructure/local/.env ] && . infrastructure/local/.env; set +a; \
	for entry in "postgres:$${PREPEET_POSTGRES_PORT:-5432}" \
	             "localstack:$${PREPEET_LOCALSTACK_PORT:-4566}" \
	             "temporal:$${PREPEET_TEMPORAL_PORT:-7233}" \
	             "temporal ui:$${PREPEET_TEMPORAL_UI_PORT:-8233}"; do \
		name=$${entry%:*}; port=$${entry##*:}; \
		if ! nc -z -G 1 127.0.0.1 "$$port" >/dev/null 2>&1 \
		   && ! nc -z -w 1 127.0.0.1 "$$port" >/dev/null 2>&1; then \
			printf "    %-12s %-6s free\n" "$$name" "$$port"; \
			continue; \
		fi; \
		container=$$(docker ps --format '{{.Names}}\t{{.Ports}}' 2>/dev/null \
			| awk -v p=":$$port->" 'index($$0, p) { print $$1; exit }'); \
		if [ -n "$$container" ]; then \
			case "$$container" in \
				prepeet-*) printf "    %-12s %-6s this stack (%s)\n" "$$name" "$$port" "$$container" ;; \
				*)         printf "    %-12s %-6s TAKEN by container %s\n" "$$name" "$$port" "$$container" ;; \
			esac; \
			continue; \
		fi; \
		process=$$(lsof -nP -iTCP:$$port -sTCP:LISTEN -F c 2>/dev/null | sed -n "s/^c//p" | head -1); \
		if [ -n "$$process" ]; then \
			printf "    %-12s %-6s TAKEN by process %s\n" "$$name" "$$port" "$$process"; \
		else \
			printf "    %-12s %-6s TAKEN, by something this user cannot see\n" "$$name" "$$port"; \
		fi; \
	done
