// Package config provides typed configuration loading from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all collector configuration values.
type Config struct {
	DatabaseURL      string        // env: DATABASE_URL (required)
	Limit            int           // env: LIMIT (default: 30) - max total products to fetch
	PageSize         int           // env: PAGE_SIZE (default: 30) - products per API page
	Workers          int           // env: WORKERS (default: 4)
	RateLimit        int           // env: RATE_LIMIT (default: 5 requests/sec)
	RequestTimeout   time.Duration // env: REQUEST_TIMEOUT (default: 10s)
	LogLevel         string        // env: LOG_LEVEL (default: "info")
	LogFormat        string        // env: LOG_FORMAT (default: "json", alt: "pretty")
	APIBaseURL       string        // env: API_BASE_URL (default: "https://dummyjson.com/products")
	MetricsAddr      string        // env: METRICS_ADDR (default: ":9090")
	MetricsKeepAlive bool          // env: METRICS_KEEP_ALIVE (default: false)
	ClickHouseDSN    string        // env: CLICKHOUSE_DSN (optional, HTTP DSN)
}

// Load reads configuration from environment variables, applies defaults,
// and validates required fields.
func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		Limit:            30,
		PageSize:         30,
		Workers:          4,
		RateLimit:        5,
		RequestTimeout:   10 * time.Second,
		LogLevel:         "info",
		LogFormat:        "json",
		APIBaseURL:       "https://dummyjson.com/products",
		MetricsAddr:      ":9090",
		MetricsKeepAlive: false,
		ClickHouseDSN:    "",
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	if v := os.Getenv("LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("LIMIT must be a positive integer, got %q", v)
		}
		cfg.Limit = n
	}

	if v := os.Getenv("PAGE_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("PAGE_SIZE must be a positive integer, got %q", v)
		}
		cfg.PageSize = n
	}

	if v := os.Getenv("WORKERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("WORKERS must be a positive integer, got %q", v)
		}
		cfg.Workers = n
	}

	if v := os.Getenv("RATE_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("RATE_LIMIT must be a positive integer, got %q", v)
		}
		cfg.RateLimit = n
	}

	if v := os.Getenv("REQUEST_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("REQUEST_TIMEOUT must be a positive duration (e.g. 10s), got %q", v)
		}
		cfg.RequestTimeout = d
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if v := os.Getenv("LOG_FORMAT"); v != "" {
		if v != "json" && v != "pretty" {
			return Config{}, fmt.Errorf("LOG_FORMAT must be \"json\" or \"pretty\", got %q", v)
		}
		cfg.LogFormat = v
	}

	if v := os.Getenv("API_BASE_URL"); v != "" {
		cfg.APIBaseURL = v
	}

	if v := os.Getenv("METRICS_ADDR"); v != "" {
		cfg.MetricsAddr = v
	}

	if v := os.Getenv("METRICS_KEEP_ALIVE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("METRICS_KEEP_ALIVE must be a boolean (true/false), got %q", v)
		}
		cfg.MetricsKeepAlive = b
	}

	if v := os.Getenv("CLICKHOUSE_DSN"); v != "" {
		cfg.ClickHouseDSN = v
	}

	return cfg, nil
}
