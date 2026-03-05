# go-collector - Improvement Plan (production-ready)

> Scope: turn the current MVP collector into a robust, testable, and interview-ready Go data pipeline.
> Baseline checked against current code on March 5, 2026.

---

## 1) Current State (validated)

Repository today:

- `cmd/collector/main.go`: env parsing inline, global timeout, sequential flow.
- `internal/fetch/fetch.go`: single HTTP GET (`?limit=N`), no pagination loop, no retries, no rate limit.
- `internal/store/store.go`: validation + transactional UPSERT with `RETURNING (xmax = 0)`.
- `internal/db/db.go`: `database/sql` + `pgx/v5/stdlib` driver.
- `migrations/001_init.sql`: `products` table without checksum/quality columns.
- No unit tests, no CI workflow, no Dockerfile.

What is already good:

- Clean Go layout (`cmd/`, `internal/`).
- Transactional UPSERT logic already present.
- Basic data quality checks already present.
- Makefile + Postgres local workflow already present.

Main gaps to close:

- Reliability: retries, error classification, cancellation behavior.
- Throughput control: pagination + worker pool + rate limiting.
- Testability: interfaces + dependency injection + unit tests.
- Operability: structured logs, metrics endpoint, graceful shutdown.
- Delivery: Docker image + CI pipeline.

---

## 2) Critical Review of the Previous Draft

Verdicts kept:

- Worker pool + rate limit: `YES` (essential).
- Retry/backoff/jitter + 429/503 handling: `YES` (essential).
- Structured logs + CI + tests: `YES` (essential).
- OpenTelemetry tracing: `NO` for now (too heavy for this project size).
- ClickHouse output: `OPTIONAL`, phase-gated.

Corrections applied to make the plan sound:

1. Proxy strategy corrected
- Do not build custom "proxy rotation" for dummyjson.
- Use standard `http.Transport{Proxy: http.ProxyFromEnvironment}`.
- Keep it simple and honest for CV.

2. Pagination strategy corrected
- DummyJSON pagination must use `skip/limit` and total count.
- First request discovers `total`, then workers fetch page jobs.

3. Retry semantics clarified
- Retry only transient failures: `429`, `503`, network errors, timeouts.
- Never retry non-transient 4xx (except `429`).
- Always close response body before retry.

4. Checksum implementation corrected
- Do checksum skip in SQL conflict clause, not with extra SELECT per row.
- Use `ON CONFLICT ... DO UPDATE ... WHERE products.checksum IS DISTINCT FROM EXCLUDED.checksum`.
- If no row returned, count as `skipped` (idempotent write).

5. Metrics implementation corrected
- Avoid package-level `promauto` globals (test collisions).
- Use a `Metrics` struct + explicit registry registration.

6. Docker runtime corrected
- Final image must include CA certs for HTTPS calls.
- Run as non-root user.

---

## 3) Target Architecture

New/updated components:

- `internal/config`: typed config loader + validation.
- `internal/logger`: structured logger factory (`zerolog`).
- `internal/fetch`: paginated concurrent client with retry + limiter.
- `internal/store`: `Store` struct + `Storer` interface + deterministic validation/checksum.
- `internal/metrics` (phase 2): Prometheus counters/histograms.
- `cmd/collector/main.go`: DI wiring + signal-based graceful shutdown.

Core interfaces:

```go
type Fetcher interface {
    FetchAll(ctx context.Context) ([]model.Product, error)
}

type Storer interface {
    UpsertProducts(ctx context.Context, products []model.Product) (Stats, error)
}
```

---

## 4) Implementation Plan

### Phase 1 - Reliable and Testable Core

### 4.1 Config package

Create `internal/config/config.go`:

- `DATABASE_URL` required.
- Defaults: `LIMIT=30`, `WORKERS=4`, `RATE_LIMIT=5`, `REQUEST_TIMEOUT=10s`, `LOG_LEVEL=info`, `LOG_FORMAT=json`, `API_BASE_URL=https://dummyjson.com/products`.
- Validation: positive numeric values, valid duration.

Deliverable:

- `Load() (Config, error)` with unit tests for defaults and invalid inputs.

### 4.2 Structured logging

Create `internal/logger/logger.go`:

- Use `zerolog`.
- `json` default output, `pretty` for local dev.
- No hidden globals in business logic.

Deliverable:

- logger passed explicitly to fetch/store/main.

### 4.3 Fetch refactor: pagination + worker pool + rate limit + retry

Refactor `internal/fetch/fetch.go`:

- Implement `Client` with injected `*http.Client`, limiter, logger.
- Use `REQUEST_TIMEOUT` as per-request timeout on the HTTP client (independent from top-level app context cancellation).
- First call gets `total`; build page jobs from `skip/limit`.
- Workers process jobs concurrently with `errgroup.WithContext`.
- Context cancellation stops all workers quickly.

Create `internal/fetch/retry.go`:

- Exponential backoff with jitter (`base=500ms`, `max=10s`, `maxRetries=3`).
- Retry policy: timeout/network/429/503 only.

Deliverable:

- `FetchAll(ctx)` returns merged products or first fatal error.

### 4.4 Store refactor: validation + idempotent UPSERT with checksum

Refactor `internal/store/store.go`:

- Introduce `Store` struct wrapping `*sql.DB` and logger.
- Extract pure `validateAndNormalize(p Product) (Product, []string, bool)` for tests.
- Add deterministic checksum on relevant business fields.
- This requires schema support in Phase 1 (see `migrations/002_quality.sql` below).

SQL approach (single roundtrip per row):

```sql
INSERT INTO products (id, title, brand, category, price, rating, stock, checksum, quality_status, quality_reasons, updated_at)
VALUES (...)
ON CONFLICT (id) DO UPDATE SET
  title = EXCLUDED.title,
  brand = EXCLUDED.brand,
  category = EXCLUDED.category,
  price = EXCLUDED.price,
  rating = EXCLUDED.rating,
  stock = EXCLUDED.stock,
  checksum = EXCLUDED.checksum,
  quality_status = EXCLUDED.quality_status,
  quality_reasons = EXCLUDED.quality_reasons,
  updated_at = now()
WHERE products.checksum IS DISTINCT FROM EXCLUDED.checksum
RETURNING (xmax = 0) AS is_insert;
```

Interpretation:

- Row returned + `is_insert=true` -> inserted.
- Row returned + `is_insert=false` -> updated.
- No row returned -> skipped (unchanged).

### 4.5 Schema migration required by checksum (moved to Phase 1)

Add migration `migrations/002_quality.sql`:

- `checksum TEXT`.
- `quality_status TEXT NOT NULL DEFAULT 'ok'`.
- `quality_reasons TEXT[] NOT NULL DEFAULT '{}'`.
- Optional index later if analytics queries require it.

### 4.6 Main refactor + graceful shutdown

Update `cmd/collector/main.go`:

- Replace ad-hoc env parsing by config loader.
- Use `signal.NotifyContext` with `SIGINT` and `SIGTERM`.
- Wire dependencies explicitly: config -> logger -> db -> fetcher -> storer.
- Exit non-zero on fatal errors with clear structured log.

### 4.7 Tests (mandatory in phase 1)

Add:

- `internal/config/config_test.go`.
- `internal/fetch/fetch_test.go` with `httptest`:
  - pagination,
  - retry (429 then 200),
  - timeout,
  - malformed JSON.
- `internal/store/store_test.go`:
  - pure validation/normalization rules,
  - checksum determinism,
  - UPSERT behavior (insert/update/skip) via integration tests against a real Postgres instance.
  - Preferred local strategy: `testcontainers-go`; CI strategy: Postgres service container in GitHub Actions.

### 4.8 Delivery files

Add:

- `.env.example`.
- `Dockerfile` (multi-stage, CA certs, non-root runtime).
- `.github/workflows/ci.yml` running `go test -race`, `go vet`, `golangci-lint`.
- Makefile targets: `demo`, `docker-build`, `ci-local`.

Phase 1 acceptance criteria:

- `go test ./... -race` passes.
- 2 consecutive runs are idempotent (`skipped` increases on second run for unchanged rows).
- SIGINT/SIGTERM stops cleanly.
- Docker image runs end-to-end with env config.

---

### Phase 2 - Observability and Quality Gates

### 5.1 Prometheus metrics

Create `internal/metrics/metrics.go`:

- Counters: inserted/updated/skipped, retries.
- Histogram: HTTP request duration by endpoint/status.
- Serve `/metrics` on configurable port (default `:9090`).

### 5.2 Schema note

- `migrations/002_quality.sql` is already delivered in Phase 1 because checksum-backed idempotency is part of core behavior.
- Phase 2 focuses on observability and quality reporting, not mandatory schema introduction.

### 5.3 Quality policy

Rules (example):

- `title == ""` -> skip.
- `price <= 0` -> skip.
- `category == ""` -> normalize to `unknown`.
- `price > 10000` -> warn.
- `stock < 0` -> warn.

Store warnings in `quality_status/quality_reasons`.

Phase 2 acceptance criteria:

- `/metrics` exported and validated locally.
- quality columns populated correctly.
- unchanged products do not trigger UPDATE writes.

---

### 6) Optional Phase 3 (only if needed for target role)

ClickHouse sink:

- Add only if job description explicitly values ClickHouse.
- Keep behind `CLICKHOUSE_DSN` feature flag.
- No impact on core collector reliability.

---

## 7) Dependencies to Add

- `github.com/rs/zerolog`
- `golang.org/x/time/rate`
- `github.com/prometheus/client_golang` (phase 2)
- `github.com/stretchr/testify` or `go.uber.org/goleak` (optional test ergonomics)
- `github.com/testcontainers/testcontainers-go` (phase 1 integration tests, optional if CI-only Postgres service is used)
- `github.com/ClickHouse/clickhouse-go/v2` (optional phase 3)

---

## 8) Verification Checklist

Automated:

```bash
go test ./... -race -count=1
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
go vet ./...
golangci-lint run ./...
docker build -t collector .
```

Manual:

1. `make demo` completes without manual intervention.
2. Logs are structured (`json` by default).
3. Running collector twice shows idempotent behavior (`skipped` for unchanged rows).
4. `curl localhost:9090/metrics` returns Prometheus metrics (phase 2).
5. GitHub Actions pipeline passes on push/PR.

---

## 9) CV Line (final)

`Go data collector: concurrent paginated ingestion (worker pool + rate limit), resilient HTTP retries with backoff/jitter, transactional PostgreSQL UPSERT with checksum-based idempotency, structured logging, Prometheus metrics, Dockerized delivery, and CI (lint/test/vet).`
