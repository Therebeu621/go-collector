// Package db provides a helper to connect to PostgreSQL.
package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // register pgx driver
)

// Connect opens a connection pool to PostgreSQL using the provided DSN
// and verifies reachability with a ping.
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	// Sensible defaults for a short-lived collector.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	return db, nil
}
