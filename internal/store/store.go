// Package store handles data quality validation and PostgreSQL upserts.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/anisse/collector/internal/model"
)

// Stats holds the result counters for an upsert batch.
type Stats struct {
	Inserted int
	Updated  int
	Skipped  int
}

// UpsertProducts validates each product and upserts valid ones into PostgreSQL.
// It returns aggregate stats and the first critical error encountered, if any.
func UpsertProducts(ctx context.Context, db *sql.DB, products []model.Product) (Stats, error) {
	var stats Stats

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() // no-op after commit

	const query = `
		INSERT INTO products (id, title, brand, category, price, rating, stock, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (id) DO UPDATE SET
			title      = EXCLUDED.title,
			brand      = EXCLUDED.brand,
			category   = EXCLUDED.category,
			price      = EXCLUDED.price,
			rating     = EXCLUDED.rating,
			stock      = EXCLUDED.stock,
			updated_at = now()
		RETURNING (xmax = 0) AS is_insert
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return stats, fmt.Errorf("preparing upsert statement: %w", err)
	}
	defer stmt.Close()

	for _, p := range products {
		// --- Data quality checks ---
		if p.Title == "" {
			log.Printf("[SKIP] product id=%d: title is empty", p.ID)
			stats.Skipped++
			continue
		}
		if p.Price <= 0 {
			log.Printf("[WARN] product id=%d %q: price=%.2f <= 0, skipping", p.ID, p.Title, p.Price)
			stats.Skipped++
			continue
		}
		if p.Category == "" {
			p.Category = "unknown"
		}

		var isInsert bool
		err := stmt.QueryRowContext(ctx, p.ID, p.Title, p.Brand, p.Category, p.Price, p.Rating, p.Stock).Scan(&isInsert)
		if err != nil {
			return stats, fmt.Errorf("upserting product id=%d: %w", p.ID, err)
		}

		if isInsert {
			stats.Inserted++
		} else {
			stats.Updated++
		}
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("committing transaction: %w", err)
	}

	return stats, nil
}
