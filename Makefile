.PHONY: install dev build lint typecheck test db-up db-down db-migrate db-seed sqlc verify verify-full db-verify migration-check migration-test generated-check security-scan ai-eval e2e e2e-clean

install:
	npm install
	python3 -m venv services/ai-research/.venv
	services/ai-research/.venv/bin/pip install -e 'services/ai-research[dev]'

dev:
	npm run dev

build:
	npm run build
	cd services/core-api && go build -o /tmp/dawha-core-api ./cmd/api

lint:
	npm run lint
	cd services/core-api && go vet ./...
	cd services/ai-research && .venv/bin/ruff check .

typecheck:
	npm run typecheck

test:
	npm run test
	cd services/core-api && go test ./...
	services/ai-research/.venv/bin/pytest

db-up:
	docker compose up -d db

db-down:
	docker compose down

db-migrate: db-up
	@infra/local/migrate.sh

db-seed:
	@docker compose exec -T db psql -U $${POSTGRES_USER:-dawha} -d $${POSTGRES_DB:-dawha} -v ON_ERROR_STOP=1 < db/seeds/001_demo.sql

sqlc:
	cd services/core-api && sqlc generate

# migration-check reads the tree and the database and writes to neither. It is
# the drift gate: every applied migration has a file, every file is applied, and
# every recorded checksum is the checksum of the file on disk. It exists because
# a green `make db-migrate` cannot tell the difference between "the database
# matches this tree" and "the runner had nothing left to do because somebody
# edited a migration after it had been applied somewhere else".
#
# A database migrated before the checksum existed has NULL checksums; `check`
# reports those as `no checksum recorded` rather than passing, and
# `make db-migrate` is what turns them into recorded backfills. Run db-migrate
# before migration-check, which is what verify-full and the CI database job do.
#
#   COMPOSE_PROJECT_NAME=dawha make migration-check
migration-check:
	@infra/local/migrate.sh check

# migration-test proves the five things the runner promises - success, failure,
# rerun, checksum and concurrent-run behaviour - against a scratch database it
# creates and drops. It is not part of `verify` because it needs PostgreSQL; it
# is part of `verify-full` and of the CI database job because the properties it
# checks are the ones no Go test can fake.
#
#   COMPOSE_PROJECT_NAME=dawha make migration-test
migration-test:
	@infra/local/migration_test.sh

# verify is the FAST gate and stays that way: lint, typecheck, unit tests and a
# build. It needs no database, no browser and no service startup, so the default
# developer loop never pays for acceptance coverage. DATABASE_URL is
# deliberately not exported here: without it the database-backed tests skip
# instead of reaching for a PostgreSQL that may not be running.
#
# It is the gate, and the gate is only worth something if it is the one everybody
# runs. `verify-full` and `make e2e` both build on what it checks, and neither
# replaces it.
verify: lint typecheck test build

# generated-check fails when the committed sqlc output no longer matches what the
# current db/migrations produce.
#
# This is not a theoretical gate. When this target was added the committed output
# was already stale: it predated the job-lease columns, the visibility columns
# and the analysis-run staging tables, so `sqlc generate` and a clean checkout
# disagreed. Nothing caught it because nothing ran `sqlc generate`. A generated
# file nobody regenerates is a comment, not a contract.
#
# The check regenerates into a copy, diffs, and restores the tree, so a failing
# check leaves the working tree exactly as it found it and the developer is not
# left with an unreviewed diff in services/core-api/generated.
#
# It needs sqlc. It is not in `verify` because it is a build-tool dependency the
# fast gate does not otherwise have, and a gate that fails because a developer
# has not installed a code generator teaches people to ignore gates.
#
#   go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
SQLC_VERSION ?= v1.31.1
generated-check:
	@if ! command -v sqlc >/dev/null 2>&1; then \
		printf 'sqlc is not installed. Install the pinned version with `go install github.com/sqlc-dev/sqlc/cmd/sqlc@%s`.\n' "$(SQLC_VERSION)" >&2; \
		exit 2; \
	fi
	@before=$$(mktemp -d) && \
		cp -R services/core-api/generated "$$before/generated" && \
		(cd services/core-api && sqlc generate) && \
		if diff -ru "$$before/generated" services/core-api/generated >/dev/null 2>&1; then \
			rm -rf "$$before"; \
			printf 'generated code is up to date with db/migrations\n'; \
		else \
			printf '%s\n' 'the committed sqlc output does not match db/migrations. Run `make sqlc` and commit the result.' >&2; \
			diff -ru "$$before/generated" services/core-api/generated 2>&1 | head -80 >&2; \
			rm -rf services/core-api/generated && cp -R "$$before/generated" services/core-api/generated; \
			rm -rf "$$before"; \
			exit 1; \
		fi

# verify-full is the ACCEPTANCE gate: the whole release signal in one command. It
# is deliberately the slow one, and it is the target that decides whether a tree
# is shippable rather than merely tidy. In order:
#
#   1. `verify` - lint, typecheck, unit tests and a build for the web app, the
#      Go service and the Python service. Included rather than assumed, because a
#      gate somebody has to remember to run in two commands is two commands.
#   2. `generated-check` - the committed sqlc output still matches the schema.
#   3. `db-migrate` - the atomic, locked, checksum-protected runner, against the
#      database named below.
#   4. `migration-check` - drift: nothing pending, every recorded checksum equal
#      to the file on disk. Run after db-migrate, which is what turns a database
#      that predates the checksum into one with recorded backfills.
#   5. `db-seed` - db/seeds/001_demo.sql, so the suite runs against the shape
#      the application actually ships with rather than an empty schema.
#   6. the COMPLETE Go suite (./...) against that database, through
#      tools/dbtestguard, which fails on a non-zero exit, on a database that was
#      never migrated, and on any test that skipped on the database gate.
#   7. `migration-test` - the runner's five cases against a scratch database.
#   8. `ai-eval` - every AI evaluation metric group.
#
# What it deliberately does NOT include, and why:
#
#   * `make e2e`. The browser suite starts a three-process stack and is its own
#     gate with its own CI job; folding it in here would make the acceptance
#     gate fail for reasons that have nothing to do with the API.
#   * `make security-scan`. The dependency and secret scanners need the network
#     and install their own tools, so putting them in the local acceptance gate
#     would turn "is this tree correct" into "is this laptop online". They are a
#     CI gate, and the same commands run locally on demand.
#   * a database reset. It migrates and seeds the database it is pointed at; it
#     never drops one. That is why POSTGRES_DB below wants a scratch database
#     when the point is to verify from clean.
#
# Environment it reads:
#   COMPOSE_PROJECT_NAME  reuse an already running `db` container. Without it
#                         `docker compose` derives the project name from the
#                         working directory and would start a SECOND PostgreSQL
#                         on the same published port 55432.
#   POSTGRES_DB           which database to migrate, seed and test. Point it at
#                         a scratch database to verify against a clean one.
#   POSTGRES_USER         default dawha
#   POSTGRES_PASSWORD     default dawha_local
#   DATABASE_URL          overrides the URL handed to the Go suite. Defaults to
#                         postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@localhost:55432/$POSTGRES_DB
#   AI_RESEARCH_URL       optional. When set, the one AI-backed Go test runs
#                         instead of skipping; the deterministic ai-research
#                         service has no credentials to configure.
verify-full: verify generated-check
	@$(MAKE) db-migrate
	@$(MAKE) migration-check
	@$(MAKE) db-seed
	cd services/core-api && \
		DATABASE_URL="$${DATABASE_URL:-postgres://$${POSTGRES_USER:-dawha}:$${POSTGRES_PASSWORD:-dawha_local}@localhost:55432/$${POSTGRES_DB:-dawha}}" \
		AI_RESEARCH_URL="$${AI_RESEARCH_URL:-}" \
		go run ./tools/dbtestguard -- ./...
	@$(MAKE) migration-test
	@$(MAKE) ai-eval

# db-verify runs the complete Go suite against DATABASE_URL with the skip audit,
# without touching migrations or the seed. This is the command the CI database
# job and verify-full both depend on. It fails when DATABASE_URL is unset, when
# the schema is missing, and when any test skipped on the database gate.
#
#   DATABASE_URL=postgres://dawha:dawha_local@localhost:55432/dawha make db-verify
db-verify:
	@if [ -z "$${DATABASE_URL:-}" ]; then \
		printf '%s\n' "db-verify needs DATABASE_URL; a database-backed suite that silently skips is exactly what this gate exists to catch" >&2; \
		exit 2; \
	fi
	cd services/core-api && go run ./tools/dbtestguard -- ./...

# ai-eval prints every configured AI quality group (routing, retrieval,
# extraction, resolution, contradiction), writes the machine-readable report,
# and exits 0 when each group's thresholds hold. There is no vector-only
# baseline in this repository, so the report says so rather than claiming a
# GraphRAG or embedding improvement.
#
#   AI_EVAL_REPORT=/tmp/dawha-eval.json make ai-eval
#
# AI_EVAL_REPORT is resolved inside services/ai-research, so the default lands
# next to the fixtures it describes.
AI_EVAL_REPORT ?= evaluation/report.json
ai-eval:
	cd services/ai-research && AI_EVAL_REPORT="$(AI_EVAL_REPORT)" .venv/bin/python -m evaluation.evaluate

# e2e builds the web app against the local API and runs the Playwright suite
# against the deterministic local stack. See apps/web/e2e/README.md and
# apps/web/playwright.config.ts for the ports and the environment each service
# needs. Nothing here is required by `verify`.
#
# The stack starts both queue consumers next to the API - source processing and the
# analysis worker - because a journey that waits for a run to finish is waiting for
# something that claims its job. `apps/web/e2e/stack.mjs` owns that list, so the
# Makefile does not repeat it and the two cannot disagree.
#
# A run removes its own rows on the way out: the Playwright global teardown
# executes apps/web/e2e/cleanup.sql, which deletes exactly what the journeys
# created and nothing else. `make e2e` therefore leaves the database as it found
# it, including the development database this target points at by default.
#
#   COMPOSE_PROJECT_NAME=dawha make db-migrate db-seed
#   COMPOSE_PROJECT_NAME=dawha make e2e
#
# COMPOSE_PROJECT_NAME reuses an already running `db` container. Point
# POSTGRES_DB at a scratch database to keep the run off the one you develop
# against; the runner uses POSTGRES_DB to build DATABASE_URL.
E2E_PORT ?= 4173
E2E_API_PORT ?= 8181
E2E_AI_PORT ?= 8182
e2e:
	@$(MAKE) db-migrate
	cd apps/web && VITE_API_URL="$${VITE_API_URL:-http://localhost:$(E2E_API_PORT)}" npm run build
	cd apps/web && \
		DATABASE_URL="$${DATABASE_URL:-postgres://$${POSTGRES_USER:-dawha}:$${POSTGRES_PASSWORD:-dawha_local}@localhost:55432/$${POSTGRES_DB:-dawha}}" \
		SOURCE_STORAGE_DIR="$${SOURCE_STORAGE_DIR:-../../.data/source-storage}" \
		CORE_API_PORT="$(E2E_API_PORT)" \
		AI_RESEARCH_PORT="$(E2E_AI_PORT)" \
		AI_RESEARCH_URL="http://localhost:$(E2E_AI_PORT)" \
		WEB_E2E_PORT="$(E2E_PORT)" \
		ANALYSIS_WORKER_POLL_INTERVAL="200ms" \
		WEB_ORIGIN="http://localhost:$(E2E_PORT)" \
		npx playwright test

# e2e-clean removes the synthetic rows a browser run created, through the same
# apps/web/e2e/cleanup.sql the Playwright teardown runs, and nothing else: only
# the e2e-*@example.invalid accounts and the rows reachable from them. It is here
# for the runs the teardown cannot clean up after - a run killed mid-flight, a
# container stopped, or the rows earlier versions of the suite left behind.
#
# The deletes are in dependency order inside one transaction, because about fifty
# tables reference users(id) without ON DELETE CASCADE, and the transaction ends
# with a proof that no row was left without its parent. A failure rolls the whole
# thing back, so the database is never left half-cleaned.
#
#   COMPOSE_PROJECT_NAME=dawha make e2e-clean
#
# It runs through the same container `make db-seed` uses, so COMPOSE_PROJECT_NAME
# matters here for the same reason it does there: without it docker compose starts
# a second PostgreSQL instead of reusing the running one.
e2e-clean:
	@$(MAKE) db-up
	@docker compose exec -T db psql -U $${POSTGRES_USER:-dawha} -d $${POSTGRES_DB:-dawha} -v ON_ERROR_STOP=1 < apps/web/e2e/cleanup.sql
