.PHONY: run test lint migrate up down

# DSN par défaut pour le dev local
DATABASE_URL ?= postgres://collector:collector@localhost:5434/collector?sslmode=disable

## Démarrer Postgres (sans docker-compose pour éviter les bugs v1)
up:
	docker run -d --name collector-pg -e POSTGRES_USER=collector -e POSTGRES_PASSWORD=collector -e POSTGRES_DB=collector -p 5434:5432 postgres:16-alpine

## Arrêter Postgres
down:
	docker rm -f collector-pg

## Appliquer la migration
migrate:
	PGPASSWORD=collector psql -h localhost -p 5434 -U collector -d collector -f migrations/001_init.sql

## Lancer le collector
run:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/collector

## Lancer les tests
test:
	go test ./... -v

## Linter (nécessite golangci-lint)
lint:
	golangci-lint run ./...
