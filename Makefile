.PHONY: run test lint migrate up down demo docker-build ci-local observability-up observability-down

COMPOSE ?= docker-compose

# DSN par défaut pour le dev local
DATABASE_URL ?= postgres://collector:collector@localhost:5434/collector?sslmode=disable

## Démarrer Postgres (idempotent : supprime l'ancien container si existant)
up:
	@docker rm -f collector-pg 2>/dev/null || true
	docker run -d --name collector-pg \
		-e POSTGRES_USER=collector \
		-e POSTGRES_PASSWORD=collector \
		-e POSTGRES_DB=collector \
		-p 5434:5432 \
		postgres:16-alpine

## Arrêter Postgres
down:
	docker rm -f collector-pg

## Appliquer toutes les migrations
migrate:
	PGPASSWORD=collector psql -h localhost -p 5434 -U collector -d collector -f migrations/001_init.sql
	PGPASSWORD=collector psql -h localhost -p 5434 -U collector -d collector -f migrations/002_quality.sql

## Lancer le collector
run:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/collector

## Lancer les tests
test:
	go test ./... -v -race -count=1

## Linter (nécessite golangci-lint)
lint:
	golangci-lint run ./...

## Vérifier le code
vet:
	go vet ./...

## Build Docker image
docker-build:
	docker build -t collector .

## One-command demo: up → wait → migrate → run
demo: up
	@echo "⏳ Waiting for Postgres to start..."
	@sleep 3
	$(MAKE) migrate
	$(MAKE) run

## CI local: lint + vet + test
ci-local: lint vet test

## Start observability stack (Prometheus + Grafana)
observability-up:
	$(COMPOSE) up -d prometheus grafana

## Stop observability stack
observability-down:
	$(COMPOSE) rm -sf prometheus grafana
