// Package store handles data quality validation and PostgreSQL upserts
// with checksum-based idempotency.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/anisse/collector/internal/metrics"
	"github.com/anisse/collector/internal/model"
)

// Storer is the interface for product persistence.
type Storer interface {
	UpsertProducts(ctx context.Context, products []model.Product) (Stats, error)
}

// Stats holds the result counters for an upsert batch.
type Stats struct {
	Inserted int
	Updated  int
	Skipped  int
}

// Store wraps the database connection and logger for product operations.
type Store struct {
	db      *sql.DB
	logger  zerolog.Logger
	metrics *metrics.Metrics // optional, may be nil
}

// New creates a Store.
func New(db *sql.DB, logger zerolog.Logger, m *metrics.Metrics) *Store {
	return &Store{db: db, logger: logger, metrics: m}
}

// UpsertProducts validates each product and upserts valid ones into PostgreSQL.
// Unchanged products (same checksum) are skipped at the SQL level.
func (s *Store) UpsertProducts(ctx context.Context, products []model.Product) (Stats, error) {
	var stats Stats

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() // no-op after commit

	const query = `
		INSERT INTO products (id, title, brand, category, price, rating, stock,
		                      checksum, quality_status, quality_reasons, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		ON CONFLICT (id) DO UPDATE SET
			title            = EXCLUDED.title,
			brand            = EXCLUDED.brand,
			category         = EXCLUDED.category,
			price            = EXCLUDED.price,
			rating           = EXCLUDED.rating,
			stock            = EXCLUDED.stock,
			checksum         = EXCLUDED.checksum,
			quality_status   = EXCLUDED.quality_status,
			quality_reasons  = EXCLUDED.quality_reasons,
			updated_at       = now()
		WHERE products.checksum IS DISTINCT FROM EXCLUDED.checksum
		RETURNING (xmax = 0) AS is_insert
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return stats, fmt.Errorf("preparing upsert statement: %w", err)
	}
	defer stmt.Close()

	for _, p := range products {
		// --- Data quality validation ---
		p, reasons, ok := ValidateAndNormalize(p)
		if !ok {
			s.logger.Warn().
				Int("product_id", p.ID).
				Str("title", p.Title).
				Strs("reasons", reasons).
				Msg("product skipped (validation failed)")
			stats.Skipped++
			if s.metrics != nil {
				s.metrics.ProductsSkipped.Inc()
			}
			continue
		}

		qualityStatus := "ok"
		if len(reasons) > 0 {
			qualityStatus = "warn"
		}
		// Ensure non-nil so pgx sends '{}' instead of NULL.
		if reasons == nil {
			reasons = []string{}
		}

		checksum := computeChecksum(p)

		var isInsert bool
		err := stmt.QueryRowContext(ctx,
			p.ID, p.Title, p.Brand, p.Category, p.Price, p.Rating, p.Stock,
			checksum, qualityStatus, reasons,
		).Scan(&isInsert)

		if err == sql.ErrNoRows {
			// Checksum unchanged — product skipped by WHERE clause.
			stats.Skipped++
			if s.metrics != nil {
				s.metrics.ProductsSkipped.Inc()
			}
			s.logger.Debug().
				Int("product_id", p.ID).
				Msg("product unchanged (checksum match)")
			continue
		}
		if err != nil {
			return stats, fmt.Errorf("upserting product id=%d: %w", p.ID, err)
		}

		if isInsert {
			stats.Inserted++
			if s.metrics != nil {
				s.metrics.ProductsInserted.Inc()
			}
		} else {
			stats.Updated++
			if s.metrics != nil {
				s.metrics.ProductsUpdated.Inc()
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("committing transaction: %w", err)
	}

	return stats, nil
}

// ValidateAndNormalize checks a product for data quality issues.
// It returns the (possibly normalized) product, a list of warning reasons,
// and a boolean indicating whether the product is valid (true = keep, false = skip).
func ValidateAndNormalize(p model.Product) (model.Product, []string, bool) {
	var reasons []string

	// Fatal: skip
	if p.Title == "" {
		return p, []string{"title is empty"}, false
	}
	if p.Price <= 0 {
		return p, []string{fmt.Sprintf("price=%.2f <= 0", p.Price)}, false
	}

	// Normalize
	if p.Category == "" {
		p.Category = "unknown"
		reasons = append(reasons, "category was empty, defaulted to unknown")
	}

	// Warnings (non-fatal)
	if p.Price > 10000 {
		reasons = append(reasons, fmt.Sprintf("price=%.2f exceeds 10000", p.Price))
	}
	if p.Stock < 0 {
		reasons = append(reasons, fmt.Sprintf("stock=%d is negative", p.Stock))
	}

	return p, reasons, true
}

// computeChecksum returns a deterministic SHA-256 hex digest of the product's
// business fields. The JSON encoding is sorted by key (Go default) which
// guarantees determinism.
func computeChecksum(p model.Product) string {
	data, _ := json.Marshal(p) // Product struct has fixed field order via json tags.
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}
