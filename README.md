# Collector — Mini-TP Go

Un collecteur de données qui récupère des produits via une API publique ([dummyjson.com](https://dummyjson.com)), valide la qualité des données, et les stocke dans PostgreSQL via un **UPSERT**.

## Arborescence

```
.
├── cmd/collector/main.go      # Point d'entrée
├── internal/
│   ├── db/db.go               # Connexion PostgreSQL (pgx)
│   ├── fetch/fetch.go         # Client HTTP + parsing JSON
│   ├── model/model.go         # Structs Go (Product, APIResponse)
│   └── store/store.go         # Validation + UPSERT transactionnel
├── migrations/001_init.sql    # DDL de la table products
├── docker-compose.yml         # Postgres 16
├── Makefile                   # Raccourcis (up, migrate, run, test, lint)
├── go.mod
└── README.md
```

## Prérequis

- **Go** ≥ 1.22
- **Docker** & **Docker Compose** (v2)
- **psql** (client PostgreSQL)

## Démarrage rapide

### 1. Lancer PostgreSQL

**Recommandé** : utiliser le Makefile qui lance Postgres via `docker run` (évite les bugs `docker-compose` v1) :

```bash
make up
```

Ou manuellement :

```bash
docker run -d --name collector-pg \
  -e POSTGRES_USER=collector \
  -e POSTGRES_PASSWORD=collector \
  -e POSTGRES_DB=collector \
  -p 5434:5432 \
  postgres:16-alpine
```

> **Note :** `docker-compose.yml` est fourni pour référence, mais le Makefile utilise `docker run` directement.

### 2. Appliquer la migration

```bash
PGPASSWORD=collector psql -h localhost -p 5434 -U collector -d collector -f migrations/001_init.sql
```

Ou via le Makefile :

```bash
make migrate
```

### 3. Installer les dépendances Go

```bash
go mod tidy
```

### 4. Lancer le collector

```bash
export DATABASE_URL="postgres://collector:collector@localhost:5434/collector?sslmode=disable"
go run ./cmd/collector
```

Ou via le Makefile :

```bash
make run
```

## Variables d'environnement

| Variable       | Obligatoire | Défaut | Description                        |
|----------------|:-----------:|--------|------------------------------------|
| `DATABASE_URL` | ✅          | —      | DSN PostgreSQL                     |
| `LIMIT`        | ❌          | `30`   | Nombre de produits à récupérer     |

## Exemple de sortie

```
2026/02/15 14:05:00 Fetching up to 30 products from API…
2026/02/15 14:05:01 Received 30 products from API
2026/02/15 14:05:01 Done — inserted: 30 | updated: 0 | skipped: 0
```

Deuxième exécution (upsert) :

```
2026/02/15 14:06:00 Fetching up to 30 products from API…
2026/02/15 14:06:01 Received 30 products from API
2026/02/15 14:06:01 Done — inserted: 0 | updated: 30 | skipped: 0
```

## Contrôles de qualité

| Règle                        | Action                              |
|------------------------------|-------------------------------------|
| `title` vide                 | Produit ignoré (log `[SKIP]`)       |
| `price` ≤ 0                 | Produit ignoré (log `[WARN]`)       |
| `category` vide              | Remplacé par `"unknown"`            |

## Vérifier les données

```bash
PGPASSWORD=collector psql -h localhost -p 5434 -U collector -d collector \
  -c "SELECT id, title, price, category FROM products ORDER BY id LIMIT 5;"
```

## Commandes Makefile

| Commande       | Description                           |
|----------------|---------------------------------------|
| `make up`      | Démarrer Postgres                     |
| `make down`    | Arrêter Postgres                      |
| `make migrate` | Appliquer la migration SQL            |
| `make run`     | Lancer le collector                   |
| `make test`    | Lancer `go test ./...`                |
| `make lint`    | Lancer `golangci-lint` (optionnel)    |

