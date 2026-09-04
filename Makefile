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
# There is no web floor here. Vitest enforces its own thresholds in
# apps/web/vitest.config.ts, and a second number in this file would be a copy
# free to drift from the one that runs. It already had: this variable said 80,
# was read by no recipe, and described a gate that is set at 95.

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

# ---------------------------------------------------------------------- build

.PHONY: build
build: build-go build-generated build-web ## Build every deployable

.PHONY: build-go
build-go: ## Compile every Go binary
	cd $(GO_DIR) && go build ./...

.PHONY: build-generated
build-generated: ## Compile the generated Go stubs, which no deployable imports yet
	@# Nothing imports the RPC stubs, so building the module is the only thing
	@# proving they compile, which is CTR-02's first criterion. Its own module,
	@# so build-go does not reach it.
	cd packages/generated/go && go build ./...

.PHONY: build-web
build-web: ## Build the Next.js production bundle
	cd $(WEB_DIR) && pnpm build

# The Python service has no build step of its own: `uv sync` resolves the
# environment and the service runs from source, so `make bootstrap-py` is the
# equivalent and there is nothing here to compile.

# ----------------------------------------------------------------------- test

.PHONY: test
test: test-go test-tools test-py test-web ## Run every fast test suite

.PHONY: test-go
test-go: ## Run the Go suite with the race detector
	cd $(GO_DIR) && go test -race ./...

# The generators are their own modules, so `go test ./...` in the service does
# not reach them and they went untested until this existed.
#
# -count=1 is load bearing rather than cautious. Their tests read the contracts
# and the documentation, which live outside the module; Go's test cache tracks
# files under the package being tested and cannot see those change, so a cached
# run reports a pass after the very drift the test exists to catch. Verified by
# deleting a schema and watching a cached run say ok.
TOOL_MODULES := tools/authzgen tools/eventgen tools/apicompat tools/vulncheck

.PHONY: test-tools
test-tools: ## Run the generators' tests, uncached
	@set -e; for module in $(TOOL_MODULES); do \
		printf 'testing %s\n' "$$module"; \
		(cd $$module && go test -count=1 ./...); \
	done

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
	cd $(GO_DIR) && go test -tags integration -timeout 20m ./...

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
#
# 20 minutes rather than 15. Each package with an integration suite starts its
# own container, SEC-02's adversarial suite in internal/isolation added another,
# and a timeout that trips is a build failure that looks like a broken test.
cover-go:
	cd $(GO_DIR) && go test -tags integration -race -timeout 10m -coverprofile=coverage.out -covermode=atomic $(GO_COVER_PKGS)
	@cd $(GO_DIR) && total=$$(../../tools/coverage/handwritten.sh coverage.out) \
		&& ../../tools/coverage/check.sh go "$$total" $(GO_COVERAGE_MIN)

.PHONY: cover-py
cover-py:
	cd $(PY_DIR) && uv run pytest --cov=prepeet_ai --cov-report=term-missing --cov-fail-under=$(PY_COVERAGE_MIN)

.PHONY: cover-web
cover-web:
	cd $(WEB_DIR) && pnpm test:coverage

# ---------------------------------------------------------------------- gates

# The gates PLT-02 requires that are not lint, coverage or contract drift. Each
# one is a target rather than a step in the workflow file, so the pipeline and
# an engineer run the same check and a change to it changes both.

.PHONY: check-boundaries
check-boundaries: ## Fail on a forbidden import or a crossed module boundary
	@# -count=1 is load bearing. The check reads the whole module's import graph
	@# through `go list`, and Go's test cache tracks only the files of the
	@# package under test, so a cached run reports a pass over an import graph it
	@# never looked at. The same trap the generators' tests hit.
	cd $(GO_DIR) && go test -count=1 ./internal/architecture/...

.PHONY: check-migrations
check-migrations: ## Apply every migration to an empty database and check what it promised
	@# Real PostgreSQL in a container, from empty. This is the only check that
	@# proves a migration applies at all: the schema the rest of the suite reads
	@# is the one these migrations produced, so a migration that fails to apply
	@# fails here first and by name rather than as an unexplained suite failure
	@# ten minutes later. It also holds the idempotency, edited-migration and
	@# row-level security tests from PLT-03, which are the same run.
	cd $(GO_DIR) && go test -tags integration -count=1 -timeout 10m ./platform/database/...

.PHONY: check-images
check-images: ## Every container base image is pinned by digest
	./tools/images/pinned.sh

.PHONY: build-images
build-images: ## Build every deployable image, reporting each digest
	@set -eu; \
	for target in api worker; do \
		printf 'building %s\n' "$$target"; \
		docker build -q -f services/platform/Dockerfile \
			--build-arg COMMAND="$$target" -t prepeet-$$target:local . ; \
	done; \
	printf 'building intelligence\n'; \
	docker build -q -f services/intelligence/Dockerfile -t prepeet-intelligence:local . ; \
	docker images --no-trunc --format '{{.Repository}}:{{.Tag}} {{.ID}}' \
		| grep -E '^prepeet-(api|worker|intelligence):local '

.PHONY: check-docs
check-docs: ## Fail if an internal documentation link does not resolve
	./tools/docs/links.sh

# ------------------------------------------------------------------ contracts

TOOLS_BIN := $(CURDIR)/.tools

# Everything a generator owns. Listed once, because a path that is generated but
# missing from here is one nobody would notice going stale.
GENERATED_PATHS := packages/generated \
	packages/generated/typescript/events.gen.ts \
	services/platform/platform/authz/catalogue.gen.go \
	services/platform/internal/candidate/db \
	services/platform/internal/content/db \
	services/platform/internal/interview/db \
	services/platform/internal/identity/db \
	services/platform/internal/notification/db \
	services/platform/platform/outbox/db \
	services/platform/platform/ratelimit/db

.PHONY: tools
tools: ## Install the pinned code generators into .tools
	@mkdir -p $(TOOLS_BIN)
	cd $(GO_DIR) && GOBIN=$(TOOLS_BIN) go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
	cd $(GO_DIR) && GOBIN=$(TOOLS_BIN) go install github.com/sqlc-dev/sqlc/cmd/sqlc
	cd $(GO_DIR) && GOBIN=$(TOOLS_BIN) go install github.com/bufbuild/buf/cmd/buf
	cd $(GO_DIR) && GOBIN=$(TOOLS_BIN) go install google.golang.org/protobuf/cmd/protoc-gen-go
	cd $(GO_DIR) && GOBIN=$(TOOLS_BIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc

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
	# The durable event catalogue, from its own contracts. Own module for the
	# same reason authzgen is: the service ships the registry and parses no
	# schema at startup.
	cd tools/eventgen && go build -o $(TOOLS_BIN)/eventgen .
	$(TOOLS_BIN)/eventgen
	gofmt -w packages/generated/go/prepeetevents
	# The repositories' SQL, checked against the real migrations rather than
	# against a description of them. See ADR-0008.
	cd $(GO_DIR) && $(TOOLS_BIN)/sqlc generate
	# The Go RPC stubs, per ADR-0004 and CTR-02.
	cd packages/contracts/rpc && PATH="$(TOOLS_BIN):$$PATH" $(TOOLS_BIN)/buf generate
	# Everything Python - messages, type stubs and the gRPC service - from
	# grpc_tools.protoc, which bundles its own protoc. buf's built-in Python
	# plugins would need one on PATH, which a clean checkout does not have.
	cd $(PY_DIR) && uv run python -m grpc_tools.protoc -I ../../packages/contracts/rpc \
		--python_out=../../packages/generated/python \
		--pyi_out=../../packages/generated/python \
		--grpc_python_out=../../packages/generated/python prepeet/intelligence/v1/intelligence.proto
	cd packages/generated/go && go mod tidy

.PHONY: check-generated
check-generated: generate ## Fail if generated code differs from the contracts
	@if [ -n "$$(git status --porcelain $(GENERATED_PATHS))" ]; then \
		echo "Generated code is out of date. Run make generate and commit the result:"; \
		git --no-pager diff --stat $(GENERATED_PATHS); \
		exit 1; \
	fi
	@printf '\033[32mPASS\033[0m generated code matches the contracts\n'

.PHONY: check-events
check-events: tools ## Fail if the event contracts would break a consumer
	@# Against the previous release rather than the previous commit, per
	@# ADR-0004, so a contract can be revised while in progress without the
	@# gate firing on every intermediate state.
	@set -e; \
	tag=$$(git describe --tags --abbrev=0 2>/dev/null || true); \
	if [ -z "$$tag" ]; then \
		printf 'no release tag yet, so there is no baseline to compare against.\n'; \
		printf 'This becomes a real gate at the first tagged release.\n'; \
		exit 0; \
	fi; \
	baseline=$$(mktemp -d); \
	trap 'rm -rf "$$baseline"' EXIT; \
	git archive "$$tag" packages/contracts/events | tar -x -C "$$baseline"; \
	cd tools/eventgen && go build -o $(TOOLS_BIN)/eventgen . && cd $(CURDIR); \
	$(TOOLS_BIN)/eventgen -baseline "$$baseline/packages/contracts/events"

.PHONY: check-rpc
check-rpc: tools ## Fail if the RPC contracts would break a deployed reader
	@# Same shape as check-events: against the previous release, per ADR-0004.
	@set -e; \
	tag=$$(git describe --tags --abbrev=0 2>/dev/null || true); \
	if [ -z "$$tag" ]; then \
		printf 'no release tag yet, so there is no baseline to compare against.\n'; \
		printf 'This becomes a real gate at the first tagged release.\n'; \
		exit 0; \
	fi; \
	cd packages/contracts/rpc && $(TOOLS_BIN)/buf breaking \
		--against "../../../.git#tag=$$tag,subdir=packages/contracts/rpc"

.PHONY: check-api
check-api: ## Fail if the HTTP contract would break a deployed client
	@# The same shape and the same reason as check-events: against the previous
	@# release rather than the previous commit, per ADR-0004, so a document can
	@# be revised while in progress without the gate firing on every
	@# intermediate state.
	@#
	@# This existed for events and for RPC and not for HTTP, which is the
	@# contract the most consumers read. Removing an endpoint, dropping a
	@# required response field or making a request field mandatory passed CI
	@# and broke a client at run time instead.
	@set -e; \
	tag=$$(git describe --tags --abbrev=0 2>/dev/null || true); \
	if [ -z "$$tag" ]; then \
		printf 'no release tag yet, so there is no baseline to compare against.\n'; \
		printf 'This becomes a real gate at the first tagged release.\n'; \
		exit 0; \
	fi; \
	baseline=$$(mktemp -d); \
	trap 'rm -rf "$$baseline"' EXIT; \
	git archive "$$tag" packages/contracts/api/openapi.yaml | tar -x -C "$$baseline"; \
	cd tools/apicompat && go build -o $(TOOLS_BIN)/apicompat . && cd $(CURDIR); \
	$(TOOLS_BIN)/apicompat "$$baseline/packages/contracts/api/openapi.yaml" \
		packages/contracts/api/openapi.yaml

.PHONY: lint-contracts
# Depends on tools for the same reason generate does: buf is a pinned
# binary in .tools, and without this the target passed anywhere somebody
# had already generated and failed on a clean checkout - which is exactly
# where CI runs it.
lint-contracts: tools ## Lint the OpenAPI document and the Protobuf module
	pnpm lint:contracts
	cd packages/contracts/rpc && $(TOOLS_BIN)/buf lint

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
	cd $(WEB_DIR) && pnpm lint && pnpm format:check && pnpm typecheck

.PHONY: fmt
fmt: ## Format everything in place
	cd $(GO_DIR) && gofmt -w .
	cd $(PY_DIR) && uv run ruff format .
	cd $(WEB_DIR) && pnpm format

# ---------------------------------------------------------------- dependencies

# The dependency audit PLT-02 asks for. Each language is audited against what it
# would deploy: the Go module, the Python runtime resolution, and the web
# production dependency set.
#
# Development tooling is deliberately not gated. A test runner's advisory is
# real but it is not reachable by anything a user talks to, and holding every
# change hostage to it is how a team learns to pass --no-audit. `make
# audit-dev-web` shows those separately, and there are findings there today.

.PHONY: audit
audit: audit-go audit-py audit-web ## Fail on a known vulnerability in what we ship

.PHONY: tools-audit
tools-audit:
	@mkdir -p $(TOOLS_BIN)
	@# Pinned through tools/vulncheck/go.mod rather than installed at @latest, so
	@# a finding cannot appear or vanish with whichever scanner a runner fetched.
	cd tools/vulncheck && GOBIN=$(TOOLS_BIN) go install golang.org/x/vuln/cmd/govulncheck
	cd tools/vulncheck && go build -o $(TOOLS_BIN)/vulncheck .

.PHONY: audit-go
audit-go: tools-audit ## Fail on a Go vulnerability this code actually calls
	@# govulncheck exits zero when it is asked for JSON, so on its own it reports
	@# rather than gates. tools/vulncheck applies the policy and says why.
	cd $(GO_DIR) && $(TOOLS_BIN)/govulncheck -format json ./... | $(TOOLS_BIN)/vulncheck

.PHONY: audit-go-verbose
audit-go-verbose: tools-audit ## Show every Go advisory with its call traces
	@# For reading rather than gating, so its exit status is ignored: govulncheck
	@# exits non-zero on the standard library findings audit-go deliberately does
	@# not gate, and this target exists to look at exactly those.
	-cd $(GO_DIR) && $(TOOLS_BIN)/govulncheck ./...

.PHONY: audit-py
# --no-deps because uv's lock has already resolved the whole graph: letting
# pip-audit resolve it again would audit a different set from the one that
# deploys. The empty-export guard is there because an export that failed quietly
# would otherwise hand pip-audit nothing, and nothing audits clean.
audit-py: ## Fail on a known vulnerability in the Python runtime dependencies
	@set -e; \
	requirements=$$(mktemp); \
	trap 'rm -f "$$requirements"' EXIT; \
	cd $(PY_DIR) && uv export --format requirements-txt --no-dev --no-emit-project \
		--no-emit-workspace --no-hashes | grep -v '^-e ' > "$$requirements"; \
	test -s "$$requirements" || { echo "the dependency export was empty, so nothing was audited"; exit 1; }; \
	uvx pip-audit@2.10.1 --disable-pip --no-deps -r "$$requirements"

.PHONY: audit-web
audit-web: ## Fail on a known vulnerability in the web production dependencies
	cd $(WEB_DIR) && pnpm audit --prod --audit-level moderate

.PHONY: audit-dev-web
audit-dev-web: ## Show advisories in the web development toolchain, which are not gated
	-cd $(WEB_DIR) && pnpm audit --audit-level moderate

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

# AGENT_ENV is what the voice interviewer needs beyond the provider keys the
# developer supplies themselves.
#
# LIVEKIT_* rather than PREPEET_LIVEKIT_*: those three names are the LiveKit
# agents SDK's own, read by its CLI before any of our code runs, so they are
# its vocabulary and not ours. The Go side keeps the prefixed pair because it
# mints grants itself. The values are the local stack's, from
# docker-compose.yml, and are not secret.
AGENT_ENV = LIVEKIT_URL="ws://localhost:$${PREPEET_LIVEKIT_PORT:-7880}" \
	LIVEKIT_API_KEY="devkey" \
	LIVEKIT_API_SECRET="devsecret-devsecret-devsecret" \
	PREPEET_API_URL="http://localhost:8080" \
	PREPEET_AGENT_TOKEN="dev-agent-token-dev-agent-token"

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
	@printf '    %-12s localhost:%s   ws, the SFU from ADR-0012\n' "livekit" "$$($(COMPOSE) port livekit 7880 | cut -d: -f2)"
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
dev: ## Start the whole stack: infrastructure, migrations, deployables, interviewer
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
		PREPEET_WEB_BASE_URL="http://localhost:3000" \
		PREPEET_S3_ENDPOINT="http://localhost:$${PREPEET_LOCALSTACK_PORT:-4566}" \
		PREPEET_LIVEKIT_URL="ws://localhost:7880" \
		PREPEET_LIVEKIT_API_KEY="devkey" \
		PREPEET_LIVEKIT_API_SECRET="devsecret-devsecret-devsecret" \
		PREPEET_LIVEKIT_API_URL="http://localhost:7880" \
		PREPEET_AGENT_TOKEN="dev-agent-token-dev-agent-token" \
		PREPEET_AUTH_ATTEMPTS_PER_ADDRESS="10" \
		PREPEET_AUTH_ATTEMPTS_PER_NETWORK="60" \
		PREPEET_AUTH_ATTEMPT_WINDOW="15m" \
		PREPEET_S3_BUCKET="prepeet-documents" \
		PREPEET_S3_ACCESS_KEY="test" PREPEET_S3_SECRET_KEY="test" \
		PREPEET_S3_PATH_STYLE="true" \
		go run ./cmd/api 2>&1 | awk '{ print "[api]    " $$0; fflush() }' ) & \
	( cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_APP_URL" PREPEET_OTLP_ENDPOINT="$$PREPEET_OTLP_ENDPOINT" \
		PREPEET_TEMPORAL_ADDRESS="localhost:$${PREPEET_TEMPORAL_PORT:-7233}" \
		PREPEET_SMTP_ADDRESS="localhost:$${PREPEET_SMTP_PORT:-1025}" \
		PREPEET_EMAIL_FROM="noreply@prepeet.local" \
		PREPEET_INTELLIGENCE_ADDRESS="localhost:50051" \
		PREPEET_S3_ENDPOINT="http://localhost:$${PREPEET_LOCALSTACK_PORT:-4566}" \
		PREPEET_S3_BUCKET="prepeet-documents" \
		PREPEET_S3_ACCESS_KEY="test" PREPEET_S3_SECRET_KEY="test" \
		PREPEET_S3_PATH_STYLE="true" \
		PREPEET_LIVEKIT_URL="ws://localhost:7880" \
		PREPEET_LIVEKIT_API_KEY="devkey" \
		PREPEET_LIVEKIT_API_SECRET="devsecret-devsecret-devsecret" \
		PREPEET_LIVEKIT_API_URL="http://localhost:7880" \
		go run ./cmd/worker 2>&1 | awk '{ print "[worker] " $$0; fflush() }' ) & \
	( cd $(WEB_DIR) && pnpm dev 2>&1 | awk '{ print "[web]    " $$0; fflush() }' ) & \
	( cd $(PY_DIR) && uv run python -m prepeet_ai.transport.server --port 50051 2>&1 \
		| awk '{ print "[intel]  " $$0; fflush() }' ) & \
	if [ -n "$$PREPEET_DEEPGRAM_API_KEY" ] && [ -n "$$PREPEET_CARTESIA_API_KEY" ]; then \
		( cd $(PY_DIR) && $(AGENT_ENV) uv run python -m prepeet_ai.agent.worker dev 2>&1 \
			| awk '{ print "[agent]  " $$0; fflush() }' ) & \
	else \
		echo "  no interviewer: PREPEET_DEEPGRAM_API_KEY and PREPEET_CARTESIA_API_KEY are unset."; \
		echo "  The room, the timeline and evaluation all work; nobody speaks or listens in"; \
		echo "  the room, so the transcript stays empty and every competency comes back"; \
		echo "  unassessed - which is the honest reading of an interview nobody conducted."; \
		echo "  Set both keys and re-run, or run 'make dev-agent' beside this stack."; \
		echo; \
	fi; \
	wait

.PHONY: dev-api
dev-api: ## Run the Go API alone
	@$(LOCAL_ENV); cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_APP_URL" \
		PREPEET_OTLP_ENDPOINT="$$PREPEET_OTLP_ENDPOINT" \
		PREPEET_WEB_BASE_URL="http://localhost:3000" \
		PREPEET_S3_ENDPOINT="http://localhost:$${PREPEET_LOCALSTACK_PORT:-4566}" \
		PREPEET_LIVEKIT_URL="ws://localhost:7880" \
		PREPEET_LIVEKIT_API_KEY="devkey" \
		PREPEET_LIVEKIT_API_SECRET="devsecret-devsecret-devsecret" \
		PREPEET_LIVEKIT_API_URL="http://localhost:7880" \
		PREPEET_AGENT_TOKEN="dev-agent-token-dev-agent-token" \
		PREPEET_AUTH_ATTEMPTS_PER_ADDRESS="10" \
		PREPEET_AUTH_ATTEMPTS_PER_NETWORK="60" \
		PREPEET_AUTH_ATTEMPT_WINDOW="15m" \
		PREPEET_S3_BUCKET="prepeet-documents" \
		PREPEET_S3_ACCESS_KEY="test" PREPEET_S3_SECRET_KEY="test" \
		PREPEET_S3_PATH_STYLE="true" go run ./cmd/api

.PHONY: dev-worker
dev-worker: ## Run the worker alone
	@# The object store and the SFU are not optional extras here: without a
	@# bucket the worker serves no candidate queue and registers neither the
	@# evidence nor the media route, so a completed session is never
	@# evaluated and the stack looks healthy while doing nothing.
	@$(LOCAL_ENV); cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_APP_URL" \
		PREPEET_TEMPORAL_ADDRESS="localhost:$${PREPEET_TEMPORAL_PORT:-7233}" \
		PREPEET_SMTP_ADDRESS="localhost:$${PREPEET_SMTP_PORT:-1025}" \
		PREPEET_EMAIL_FROM="noreply@prepeet.local" \
		PREPEET_INTELLIGENCE_ADDRESS="localhost:50051" \
		PREPEET_S3_ENDPOINT="http://localhost:$${PREPEET_LOCALSTACK_PORT:-4566}" \
		PREPEET_S3_BUCKET="prepeet-documents" \
		PREPEET_S3_ACCESS_KEY="test" PREPEET_S3_SECRET_KEY="test" \
		PREPEET_S3_PATH_STYLE="true" \
		PREPEET_LIVEKIT_URL="ws://localhost:7880" \
		PREPEET_LIVEKIT_API_KEY="devkey" \
		PREPEET_LIVEKIT_API_SECRET="devsecret-devsecret-devsecret" \
		PREPEET_LIVEKIT_API_URL="http://localhost:7880" go run ./cmd/worker

.PHONY: dev-web
dev-web: ## Run the Next.js application alone
	cd $(WEB_DIR) && pnpm dev

.PHONY: dev-agent
dev-agent: ## Run the voice interviewer alone (needs Deepgram and Cartesia keys)
	@$(LOCAL_ENV); \
	if [ -z "$$PREPEET_DEEPGRAM_API_KEY" ] || [ -z "$$PREPEET_CARTESIA_API_KEY" ]; then \
		echo "  The interviewer needs both PREPEET_DEEPGRAM_API_KEY and"; \
		echo "  PREPEET_CARTESIA_API_KEY. Without them it joins the room and neither"; \
		echo "  speaks nor hears, which is a silence that looks like a bug."; \
		echo; \
		echo "  Set them and try again:"; \
		echo "    PREPEET_DEEPGRAM_API_KEY=... PREPEET_CARTESIA_API_KEY=... make dev-agent"; \
		exit 1; \
	fi; \
	cd $(PY_DIR) && $(AGENT_ENV) uv run python -m prepeet_ai.agent.worker dev

.PHONY: migrate
migrate: ## Apply database migrations to the local stack
	@$(LOCAL_ENV); cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_MIGRATOR_URL" \
		PREPEET_APP_DATABASE_PASSWORD="app-password" go run ./cmd/migrate

.PHONY: publish-content
publish-content: ## Publish git-authored artifacts into the local registry (idempotent)
	@$(LOCAL_ENV); cd $(GO_DIR) && PREPEET_DATABASE_URL="$$PREPEET_APP_URL" \
		PREPEET_CONTENT_AUTHOR="$${PREPEET_CONTENT_AUTHOR:-00000000-0000-7000-8000-0000000000c9}" \
		PREPEET_CONTENT_PUBLISHER="$${PREPEET_CONTENT_PUBLISHER:-00000000-0000-7000-8000-0000000000ca}" \
		go run ./cmd/contentctl -dir ../intelligence/artifacts

# ------------------------------------------------------------------------- ci

.PHONY: ci
# Cheapest first, so a run that is going to fail says so before it has spent ten
# minutes in containers. The order is the pipeline's own reasoning, not a
# preference: lint and boundaries need no services, the contract and migration
# gates need a toolchain and a database, and coverage runs the whole suite.
#
# What this does not include is the browser job. Its appearance baselines are
# per operating system, so running it on a laptop compares macOS rendering
# against images generated on Linux and fails for a reason that has nothing to
# do with the change. `make test-browser` runs it deliberately.
ci: lint check-docs check-boundaries check-generated check-events check-rpc check-api build test-tools check-migrations cover audit ## Everything CI runs, in the order CI runs it
