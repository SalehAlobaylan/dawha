.PHONY: install dev build lint typecheck test db-up db-down db-migrate db-seed sqlc verify ai-eval

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

verify: lint typecheck test build

ai-eval:
	cd services/ai-research && .venv/bin/python -m evaluation.evaluate_routing
