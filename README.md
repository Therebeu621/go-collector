# Collector — Pipeline de données Go (orienté production)

Un collecteur de données écrit en Go qui récupère des produits depuis une API publique ([dummyjson.com](https://dummyjson.com)), valide la qualité des données, et les stocke dans PostgreSQL avec des upserts idempotents basés sur un checksum.

## Fonctionnalités

- **Pagination concurrente** — worker pool avec `errgroup` pour fetcher les pages en parallèle
- **Rate limiting** — token bucket (`golang.org/x/time/rate`) pour respecter les limites API
- **HTTP résilient** — backoff exponentiel avec jitter, retry sur 429/503/timeouts
- **Idempotence par checksum** — SHA-256 pour ignorer les lignes inchangées au niveau SQL
- **Contrôle qualité** — règles de validation avec `quality_status` (ok/warn) et raisons
- **Logs structurés** — sortie JSON via `zerolog` (mode pretty pour le dev local)
- **Arrêt gracieux** — `SIGINT`/`SIGTERM` annule proprement le travail en cours
- **Métriques Prometheus** — endpoint `/metrics` exposé sur `METRICS_ADDR`
- **Dashboard Grafana auto-provisionné** — datasource + dashboard importés automatiquement au démarrage
- **Sink ClickHouse optionnel** — export batch des produits vers ClickHouse si `CLICKHOUSE_DSN` est défini
- **Support proxy** — standard `HTTP_PROXY`/`HTTPS_PROXY` via `http.ProxyFromEnvironment`
- **Dockerisé** — build multi-stage, certificats CA, utilisateur non-root
- **CI** — GitHub Actions avec `golangci-lint`, `go test -race`, `go vet`

## Positionnement du projet

Ce projet est un **mini pipeline data orienté production** : il montre des patterns solides (fiabilité HTTP, idempotence, observabilité, CI, containerisation).

Il ne prétend pas couvrir tout un contexte enterprise (multi-environnements, gestion avancée des secrets, haute disponibilité, SLO/SLA, etc.).

## Architecture

```
.
├── cmd/collector/main.go          # Point d'entrée : config → logger → db → fetch → store
├── internal/
│   ├── config/config.go           # Config typée depuis les variables d'env + validation
│   ├── logger/logger.go           # Factory zerolog (json/pretty)
│   ├── fetch/
│   │   ├── fetch.go               # Client paginé, worker pool, rate limiter
│   │   └── retry.go               # Backoff exponentiel + jitter + classification d'erreurs
│   ├── db/db.go                   # Connexion PostgreSQL (pgx)
│   ├── model/model.go             # Structs Product + APIResponse
│   ├── store/store.go             # Validation + checksum + UPSERT transactionnel
│   └── analytics/clickhouse.go    # Export optionnel vers ClickHouse (HTTP)
├── migrations/
│   ├── 001_init.sql               # Table products
│   └── 002_quality.sql            # Colonnes checksum + qualité
├── Dockerfile                     # Multi-stage (golang → alpine)
├── docker-compose.yml             # Services Postgres + ClickHouse + collector + observabilité
├── monitoring/
│   ├── prometheus/prometheus.yml  # Jobs de scrape Prometheus
│   └── grafana/                   # Provisioning datasource + dashboard
├── .github/workflows/ci.yml       # Pipeline CI lint + test
├── .env.example                   # Variables d'env documentées
├── Makefile                       # Raccourcis dev
└── README.md
```

## Prérequis

- **Go** ≥ 1.24
- **Docker** + Docker Compose (`docker compose` v2 ou `docker-compose` v1)
- **psql** (client PostgreSQL)

## Démarrage rapide

### Commande unique

```bash
make demo    # lance Postgres, applique les migrations, exécute le collector
```

### Étape par étape

```bash
# 1. Démarrer Postgres
make up

# 2. Appliquer les migrations
make migrate

# 3. Lancer le collector
make run

# 4. Arrêter Postgres
make down
```

### Avec Docker Compose

```bash
docker compose up --build
```

Si `docker compose` n'est pas disponible (plugin v2 absent), utilise `docker-compose`:

```bash
docker-compose up --build
```

## Observabilité (Prometheus + Grafana)

```bash
# Avec docker-compose v1 (par défaut du Makefile)
make observability-up

# Si besoin, forcer docker compose v2
make observability-up COMPOSE="docker compose"
```

Endpoints :
- Prometheus : `http://localhost:9091`
- Grafana : `http://localhost:3000` (login par défaut `admin` / `admin`)
- Credentials Grafana personnalisables via `.env` : `GRAFANA_ADMIN_USER`, `GRAFANA_ADMIN_PASSWORD`

Si tu lances le collector en local et veux garder `/metrics` actif après la collecte :

```bash
METRICS_KEEP_ALIVE=true make run
```

Dashboard provisionné :
- `Collector Overview` (folder `Collector`)

### Aperçu Grafana

![Dashboard Grafana Collector](docs/images/grafana-dashboard.png)

## Variables d'environnement

| Variable | Obligatoire | Défaut | Description |
|---|:---:|---|---|
| `DATABASE_URL` | ✅ | — | DSN PostgreSQL |
| `LIMIT` | ❌ | `30` | Nombre max de produits à récupérer |
| `PAGE_SIZE` | ❌ | `30` | Produits par page API (pagination) |
| `WORKERS` | ❌ | `4` | Workers de fetch concurrents |
| `RATE_LIMIT` | ❌ | `5` | Requêtes max par seconde |
| `REQUEST_TIMEOUT` | ❌ | `10s` | Timeout HTTP par requête |
| `LOG_LEVEL` | ❌ | `info` | Niveau de log (debug/info/warn/error) |
| `LOG_FORMAT` | ❌ | `json` | Format de log (`json` ou `pretty`) |
| `API_BASE_URL` | ❌ | `https://dummyjson.com/products` | Endpoint API |
| `HTTP_PROXY` | ❌ | — | URL du proxy HTTP |
| `HTTPS_PROXY` | ❌ | — | URL du proxy HTTPS |
| `METRICS_ADDR` | ❌ | `:9090` | Adresse du serveur de métriques Prometheus |
| `METRICS_KEEP_ALIVE` | ❌ | `false` | Garde le process actif après la collecte (utile pour debug Grafana/Prometheus) |
| `CLICKHOUSE_DSN` | ❌ | — | DSN HTTP ClickHouse (ex: `http://collector:collector@localhost:8123/default`) |

## Exemple de sortie

Première exécution (logs JSON structurés) :
```json
{"level":"info","limit":30,"workers":4,"rate_limit":5,"api_url":"https://dummyjson.com/products","time":"...","message":"starting collector"}
{"level":"info","total":194,"limit":30,"workers":4,"time":"...","message":"discovered total products, starting pagination"}
{"level":"info","count":30,"time":"...","message":"products fetched from API"}
{"level":"info","inserted":30,"updated":0,"skipped":0,"time":"...","message":"collection complete"}
```

Deuxième exécution (idempotence par checksum) :
```json
{"level":"info","inserted":0,"updated":0,"skipped":30,"time":"...","message":"collection complete"}
```

## Contrôles de qualité

| Règle | Action |
|---|---|
| `title` vide | Produit **ignoré** |
| `price` ≤ 0 | Produit **ignoré** |
| `category` vide | Normalisé à `"unknown"` (warn) |
| `price` > 10000 | Conservé avec `quality_status = "warn"` |
| `stock` < 0 | Conservé avec `quality_status = "warn"` |

## Tests

```bash
# Lancer tous les tests
make test

# Avec détection de race conditions
go test ./... -race -v -count=1

# Couverture
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

## Validation ClickHouse (E2E)

```bash
# Nettoyage
docker rm -f collector-pg collector-ch collector-app 2>/dev/null || true
docker volume rm testgo_ch_data 2>/dev/null || true

# Postgres (recommande via Makefile, plus stable avec docker-compose v1)
make up
sleep 3
make migrate

# ClickHouse
docker-compose up -d clickhouse
sleep 5

# Verifications infra
PGPASSWORD=collector psql -h localhost -p 5434 -U collector -d collector -c "select 1;"
curl -u collector:collector http://localhost:8123/ping

# Run collector avec export analytics
CLICKHOUSE_DSN=http://collector:collector@localhost:8123/default make run

# Verifier le nombre de lignes exportees
curl -u collector:collector "http://localhost:8123/?query=SELECT%20count()%20FROM%20default.product_prices"
```

## Commandes Makefile

| Commande | Description |
|---|---|
| `make up` | Démarrer le conteneur Postgres |
| `make down` | Arrêter le conteneur Postgres |
| `make migrate` | Appliquer toutes les migrations SQL |
| `make run` | Lancer le collector |
| `make test` | Lancer les tests avec `-race` |
| `make lint` | Lancer `golangci-lint` |
| `make vet` | Lancer `go vet` |
| `make demo` | Commande unique : up + migrate + run |
| `make docker-build` | Construire l'image Docker |
| `make ci-local` | Lancer lint + vet + test en local |
| `make observability-up` | Démarrer Prometheus + Grafana |
| `make observability-down` | Supprimer Prometheus + Grafana |

Pour utiliser `docker compose` v2 avec ces targets :

```bash
make observability-up COMPOSE="docker compose"
```

## Vérifier les données

```bash
PGPASSWORD=collector psql -h localhost -p 5434 -U collector -d collector \
  -c "SELECT id, title, price, quality_status, checksum FROM products ORDER BY id LIMIT 5;"
```
