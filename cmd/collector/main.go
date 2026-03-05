// Collector fetches products from a public API and upserts them into PostgreSQL.
// It supports concurrent pagination, rate limiting, retries, and graceful shutdown.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anisse/collector/internal/analytics"
	"github.com/anisse/collector/internal/config"
	"github.com/anisse/collector/internal/db"
	"github.com/anisse/collector/internal/fetch"
	"github.com/anisse/collector/internal/logger"
	"github.com/anisse/collector/internal/metrics"
	"github.com/anisse/collector/internal/model"
	"github.com/anisse/collector/internal/store"
)

func main() {
	// 1. Load configuration.
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString("fatal: " + err.Error() + "\n")
		os.Exit(1)
	}

	// 2. Setup structured logger.
	log := logger.New(cfg.LogLevel, cfg.LogFormat)

	// 3. Graceful shutdown via signal.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 4. Start Prometheus metrics server.
	m := metrics.New()
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	srv := &http.Server{Addr: cfg.MetricsAddr, Handler: mux}

	go func() {
		log.Info().Str("addr", cfg.MetricsAddr).Msg("metrics server started")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("metrics server failed")
		}
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// 5. Connect to database.
	conn, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("database connection failed")
	}
	defer conn.Close()

	// 6. Create pipeline components.
	httpClient := &http.Client{
		Timeout:   cfg.RequestTimeout,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
	}

	fetcher := fetch.NewClient(
		cfg.APIBaseURL,
		cfg.Limit,
		cfg.PageSize,
		cfg.Workers,
		cfg.RateLimit,
		httpClient,
		log,
		m,
	)

	storer := store.New(conn, log, m)

	var clickhouseWriter *analytics.ClickHouseWriter
	if cfg.ClickHouseDSN != "" {
		writer, err := analytics.NewClickHouseWriter(ctx, cfg.ClickHouseDSN, log)
		if err != nil {
			log.Error().Err(err).Msg("clickhouse initialization failed, continuing without analytics sink")
		} else {
			clickhouseWriter = writer
			defer func() {
				_ = clickhouseWriter.Close()
			}()
			log.Info().Msg("clickhouse sink enabled")
		}
	}

	// 7. Execute pipeline: fetch → validate → upsert.
	log.Info().
		Int("limit", cfg.Limit).
		Int("workers", cfg.Workers).
		Int("rate_limit", cfg.RateLimit).
		Str("api_url", cfg.APIBaseURL).
		Msg("starting collector")

	products, err := fetcher.FetchAll(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("fetch failed")
	}
	log.Info().Int("count", len(products)).Msg("products fetched from API")

	stats, err := storer.UpsertProducts(ctx, products)
	if err != nil {
		log.Fatal().Err(err).Msg("upsert failed")
	}

	if clickhouseWriter != nil {
		analyticsProducts := make([]model.Product, 0, len(products))
		for _, p := range products {
			normalized, _, ok := store.ValidateAndNormalize(p)
			if !ok {
				continue
			}
			analyticsProducts = append(analyticsProducts, normalized)
		}

		exported, err := clickhouseWriter.WriteProducts(ctx, analyticsProducts)
		if err != nil {
			log.Error().Err(err).Msg("clickhouse export failed")
		} else {
			log.Info().Int("rows", exported).Msg("clickhouse export complete")
		}
	}

	// 8. Report.
	log.Info().
		Int("inserted", stats.Inserted).
		Int("updated", stats.Updated).
		Int("skipped", stats.Skipped).
		Msg("collection complete")
}
