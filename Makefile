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
test: test-go test-py test-web ## Run every fast test suite

.PHONY: test-go
test-go: ## Run the Go suite with the race detector
	cd $(GO_DIR) && go test -race ./...

.PHONY: test-py
test-py: ## Run the Python suite
	cd $(PY_DIR) && uv run pytest

.PHONY: test-web
test-web: ## Run the web suite
	cd $(WEB_DIR) && pnpm test

.PHONY: test-browser
test-browser: ## Run the browser tests: accessibility, layout and appearance
	cd $(WEB_DIR) && pnpm test:browser

.PHONY: test-browser-update
test-browser-update: ## Accept the current appearance as the new baseline
	@echo "  This rewrites the committed screenshots. Review the image diff, not"
	@echo "  just the test result: an accepted baseline is a decision about how"
	@echo "  the product looks."
	cd $(WEB_DIR) && pnpm test:browser:update

.PHONY: test-browser-baselines
test-browser-baselines: ## Regenerate Linux baselines in the container CI uses
	@# Screenshots differ between operating systems, so Playwright names them per
	@# platform. Baselines taken on macOS cannot be compared on a Linux runner,
	@# and generating them there by hand is how they end up stale.
	@#
	@# This exports a clean copy of the tree and runs the container against that,
	@# copying only the resulting images back. Mounting the working tree was
	@# tried twice and broke the host both times: the container installs Linux
	@# binaries, and every local command then fails with a native module error
	@# until somebody reinstalls. Shadowing node_modules with volumes was not
	@# enough. A container that cannot reach the working tree cannot damage it,
	@# which is a shorter argument than getting the mounts right.
	@# The working tree, not HEAD. An earlier version archived the last commit,
	@# which produced baselines of the committed interface using the current
	@# specs: a mixture that matches nothing, and silently, because a baseline
	@# has no way to say which code it came from.
	@set -e; \
	export_dir=$$(mktemp -d); \
	trap 'rm -rf "$$export_dir"' EXIT; \
	tar -c --exclude=node_modules --exclude=.next --exclude=test-results \
		--exclude=playwright-report --exclude=.git . | tar -x -C "$$export_dir"; \
	docker run --rm --network host \
		-v "$$export_dir":/work -w /work/$(WEB_DIR) \
		-e CI=1 \
		mcr.microsoft.com/playwright:v1.62.1-noble \
		sh -c "corepack enable && cd /work && pnpm install --frozen-lockfile && cd $(WEB_DIR) && pnpm test:browser:update"; \
	cp "$$export_dir/$(WEB_DIR)/e2e/visual.spec.ts-snapshots"/*linux.png \
		$(WEB_DIR)/e2e/visual.spec.ts-snapshots/; \
	echo "  Linux baselines copied. Review the images before committing."

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

# Everything a generator owns. Listed once, because a path that is generated but
# missing from here is one nobody would notice going stale.
GENERATED_PATHS := packages/generated \
	services/platform/platform/authz/catalogue.gen.go \
	services/platform/internal/identity/db \
	services/platform/platform/outbox/db \
	services/platform/platform/ratelimit/db

.PHONY: tools
tools: ## Install the pinned code generators into .tools
	@mkdir -p $(TOOLS_BIN)
	cd $(GO_DIR) && GOBIN=$(TOOLS_BIN) go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
	cd $(GO_DIR) && GOBIN=$(TOOLS_BIN) go install github.com/sqlc-dev/sqlc/cmd/sqlc

.PHONY: generate
generate: tools ## Regenerate every client and server type from the contracts
	# Generation runs from the repository root, because the generator resolves
	# output paths against the working directory rather than against its config.
	$(TOOLS_BIN)/oapi-codegen -config packages/contracts/api/oapi-codegen.yaml packages/contracts/api/openapi.yaml
	pnpm generate:api
	# The capability catalogue, from its own contract. Its own module, so a
	# build-time YAML parser does not become a runtime dependency of the service.
	cd tools/authzgen && go build -o $(TOOLS_BIN)/authzgen .
	$(TOOLS_BIN)/authzgen
	cd $(GO_DIR) && gofmt -w platform/authz
	# The repositories' SQL, checked against the real migrations rather than
	# against a description of them. See ADR-0008.
	cd $(GO_DIR) && $(TOOLS_BIN)/sqlc generate
	cd packages/generated/go && go mod tidy

.PHONY: check-generated
check-generated: generate ## Fail if generated code differs from the contracts
	@if [ -n "$$(git status --porcelain $(GENERATED_PATHS))" ]; then \
		echo "Generated code is out of date. Run make generate and commit the result:"; \
		git --no-pager diff --stat $(GENERATED_PATHS); \
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

# LOCAL_ENV loads the same port overrides the compose file reads.
#
# Every dev target starts with this. Without it the application would connect to
# the default port while compose published whichever one the developer moved it
# to, and the two would disagree in a way that reads as "the database is down"
# rather than as "you changed a port".
#
# The credentials are the local ones from docker-compose.yml and are not secret.
# A deployed environment supplies its own through PLT-07.
LOCAL_ENV = set -a; [ -f infrastructure/local/.env ] && . infrastructure/local/.env; set +a; \
	PORT=$${PREPEET_POSTGRES_PORT:-5432}; \
	export PREPEET_APP_URL="postgres://prepeet_app:app-password@localhost:$$PORT/prepeet?sslmode=disable"; \
	export PREPEET_MIGRATOR_URL="postgres://prepeet:local-development-only@localhost:$$PORT/prepeet?sslmode=disable"; \
	export PREPEET_OTLP_ENDPOINT="localhost:$${PREPEET_OTLP_PORT:-4317}"

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
	@printf '    %-12s localhost:%s   traces from api and worker\n' "jaeger" "$$($(COMPOSE) port jaeger 16686 | cut -d: -f2)"
	@printf '    %-12s localhost:%s   metrics, scrapeable with curl\n' "otel metrics" "$$($(COMPOSE) port otel-collector 8889 | cut -d: -f2)"
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
	             "temporal ui:$${PREPEET_TEMPORAL_UI_PORT:-8233}" \
	             "jaeger:$${PREPEET_JAEGER_UI_PORT:-16686}" \
	             "otlp:$${PREPEET_OTLP_PORT:-4317}" \
	             "otel metrics:$${PREPEET_METRICS_PORT:-8889}"; do \
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

# ------------------------------------------------------------------ run local

.PHONY: dev
dev: ## Start the whole stack: infrastructure, migrations, and all three deployables
	@$(MAKE) --no-print-directory local-up
	@$(MAKE) --no-print-directory migrate
	@echo
	@echo "  starting api, worker and web. Ctrl-C stops all three."
	@echo
	@# Run concurrently and stop together. Without the trap, Ctrl-C leaves
	@# background processes holding ports, and the next `make dev` fails with a
	@# bind error that says nothing about why.
	@#
	@# awk with fflush rather than sed, because sed block-buffers when its
	@# output is not a terminal. `make dev > log` then shows the stack starting
	@# and nothing after it, which reads as three processes that hung.
	@$(LOCAL_ENV); \
	trap 'kill 0' INT TERM EXIT; \
	( cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_APP_URL" PREPEET_OTLP_ENDPOINT="$$PREPEET_OTLP_ENDPOINT" \
		go run ./cmd/api 2>&1 | awk '{ print "[api]    " $$0; fflush() }' ) & \
	( cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_APP_URL" PREPEET_OTLP_ENDPOINT="$$PREPEET_OTLP_ENDPOINT" \
		PREPEET_TEMPORAL_ADDRESS="localhost:$${PREPEET_TEMPORAL_PORT:-7233}" \
		go run ./cmd/worker 2>&1 | awk '{ print "[worker] " $$0; fflush() }' ) & \
	( cd $(WEB_DIR) && pnpm dev 2>&1 | awk '{ print "[web]    " $$0; fflush() }' ) & \
	wait

.PHONY: dev-api
dev-api: ## Run the Go API alone
	@$(LOCAL_ENV); cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_APP_URL" \
		PREPEET_OTLP_ENDPOINT="$$PREPEET_OTLP_ENDPOINT" go run ./cmd/api

.PHONY: dev-worker
dev-worker: ## Run the worker alone
	@$(LOCAL_ENV); cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_APP_URL" \
		PREPEET_TEMPORAL_ADDRESS="localhost:$${PREPEET_TEMPORAL_PORT:-7233}" go run ./cmd/worker

.PHONY: dev-web
dev-web: ## Run the Next.js application alone
	cd $(WEB_DIR) && pnpm dev

.PHONY: migrate
migrate: ## Apply database migrations to the local stack
	@$(LOCAL_ENV); cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_MIGRATOR_URL" \
		PREPEET_APP_DATABASE_PASSWORD="app-password" go run ./cmd/migrate

# ------------------------------------------------------------------------- ci

.PHONY: ci
ci: lint check-generated cover ## Everything CI runs, in the order CI runs it
