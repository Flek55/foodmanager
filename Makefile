# Commands work in both cmd.exe and sh: no `cd`, only -C / --prefix flags.

.PHONY: help db-up db-down db-reset db-psql run web gen build test check

help:
	@echo db-up     - start dev Postgres (localhost:5433)
	@echo db-down   - stop dev Postgres, keep data
	@echo db-reset  - stop dev Postgres and DELETE its data
	@echo db-psql   - open psql shell in dev database
	@echo run       - run Go backend on :8080 (applies migrations)
	@echo web       - run Vite dev server on :5173
	@echo gen       - regenerate code: sqlc queries, Go server and TS client from openapi.yaml
	@echo build     - production build of frontend and backend
	@echo test      - run all tests
	@echo check     - vet + tests + typecheck, same as CI

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

db-reset:
	docker compose down -v

db-psql:
	docker compose exec postgres psql -U foodmanager -d foodmanager

run:
	go -C backend run ./cmd/server

web:
	npm --prefix frontend run dev

SQLC_IMAGE = sqlc/sqlc:1.30.0

gen:
	docker run --rm -v "$(CURDIR)/backend:/src" -w /src $(SQLC_IMAGE) generate
	go -C backend generate ./...
	npm --prefix frontend run gen:api

build:
	npm --prefix frontend run build
	go -C backend build -o ../bin/server ./cmd/server

test:
	go -C backend test ./...

check:
	go -C backend vet ./...
	go -C backend test ./...
	npm --prefix frontend run typecheck
