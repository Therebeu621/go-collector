package config

import (
	"os"
	"testing"
)

// setEnv is a test helper that sets an env var and registers cleanup.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func TestLoad_Defaults(t *testing.T) {
	setEnv(t, "DATABASE_URL", "postgres://test:test@localhost/test")

	// Clear optional vars to test defaults.
	for _, k := range []string{"LIMIT", "PAGE_SIZE", "WORKERS", "RATE_LIMIT", "REQUEST_TIMEOUT", "LOG_LEVEL", "LOG_FORMAT", "API_BASE_URL"} {
		os.Unsetenv(k)
	}

	cfg, err := config_load(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Limit != 30 {
		t.Errorf("Limit = %d, want 30", cfg.Limit)
	}
	if cfg.PageSize != 30 {
		t.Errorf("PageSize = %d, want 30", cfg.PageSize)
	}
	if cfg.Workers != 4 {
		t.Errorf("Workers = %d, want 4", cfg.Workers)
	}
	if cfg.RateLimit != 5 {
		t.Errorf("RateLimit = %d, want 5", cfg.RateLimit)
	}
	if cfg.RequestTimeout.String() != "10s" {
		t.Errorf("RequestTimeout = %s, want 10s", cfg.RequestTimeout)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
	if cfg.APIBaseURL != "https://dummyjson.com/products" {
		t.Errorf("APIBaseURL = %q, want default", cfg.APIBaseURL)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	os.Unsetenv("DATABASE_URL")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}

func TestLoad_InvalidLimit(t *testing.T) {
	setEnv(t, "DATABASE_URL", "postgres://test:test@localhost/test")

	tests := []struct {
		name  string
		value string
	}{
		{"negative", "-5"},
		{"zero", "0"},
		{"non-numeric", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, "LIMIT", tt.value)
			_, err := Load()
			if err == nil {
				t.Errorf("expected error for LIMIT=%q", tt.value)
			}
		})
	}
}

func TestLoad_InvalidWorkers(t *testing.T) {
	setEnv(t, "DATABASE_URL", "postgres://test:test@localhost/test")
	setEnv(t, "WORKERS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for WORKERS=0")
	}
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	setEnv(t, "DATABASE_URL", "postgres://test:test@localhost/test")
	setEnv(t, "LOG_FORMAT", "xml")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for LOG_FORMAT=xml")
	}
}

func TestLoad_InvalidRequestTimeout(t *testing.T) {
	setEnv(t, "DATABASE_URL", "postgres://test:test@localhost/test")
	setEnv(t, "REQUEST_TIMEOUT", "not-a-duration")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid REQUEST_TIMEOUT")
	}
}

func TestLoad_InvalidPageSize(t *testing.T) {
	setEnv(t, "DATABASE_URL", "postgres://test:test@localhost/test")

	tests := []struct {
		name  string
		value string
	}{
		{"negative", "-1"},
		{"zero", "0"},
		{"non-numeric", "xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, "PAGE_SIZE", tt.value)
			_, err := Load()
			if err == nil {
				t.Errorf("expected error for PAGE_SIZE=%q", tt.value)
			}
		})
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setEnv(t, "DATABASE_URL", "postgres://custom:custom@db:5432/mydb")
	setEnv(t, "LIMIT", "100")
	setEnv(t, "PAGE_SIZE", "50")
	setEnv(t, "WORKERS", "8")
	setEnv(t, "RATE_LIMIT", "10")
	setEnv(t, "REQUEST_TIMEOUT", "30s")
	setEnv(t, "LOG_LEVEL", "debug")
	setEnv(t, "LOG_FORMAT", "pretty")
	setEnv(t, "API_BASE_URL", "https://api.example.com/products")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://custom:custom@db:5432/mydb" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.Limit != 100 {
		t.Errorf("Limit = %d, want 100", cfg.Limit)
	}
	if cfg.PageSize != 50 {
		t.Errorf("PageSize = %d, want 50", cfg.PageSize)
	}
	if cfg.Workers != 8 {
		t.Errorf("Workers = %d, want 8", cfg.Workers)
	}
	if cfg.RateLimit != 10 {
		t.Errorf("RateLimit = %d, want 10", cfg.RateLimit)
	}
	if cfg.RequestTimeout.String() != "30s" {
		t.Errorf("RequestTimeout = %s, want 30s", cfg.RequestTimeout)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.LogFormat != "pretty" {
		t.Errorf("LogFormat = %q, want pretty", cfg.LogFormat)
	}
	if cfg.APIBaseURL != "https://api.example.com/products" {
		t.Errorf("APIBaseURL = %q", cfg.APIBaseURL)
	}
}

// config_load is a helper that calls Load() — just for readability.
func config_load(_ *testing.T) (Config, error) {
	return Load()
}
