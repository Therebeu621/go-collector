package analytics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/anisse/collector/internal/model"
)

func TestParseHTTPDSN(t *testing.T) {
	tests := []struct {
		name        string
		dsn         string
		wantBaseURL string
		wantDB      string
		wantUser    string
		wantPass    string
		wantErr     bool
	}{
		{
			name:        "http dsn",
			dsn:         "http://default:secret@localhost:8123/analytics",
			wantBaseURL: "http://localhost:8123/",
			wantDB:      "analytics",
			wantUser:    "default",
			wantPass:    "secret",
		},
		{
			name:        "clickhouse dsn fallback to http",
			dsn:         "clickhouse://default:@localhost/default",
			wantBaseURL: "http://localhost:8123/",
			wantDB:      "default",
			wantUser:    "default",
			wantPass:    "",
		},
		{
			name:        "clickhouse dsn with native port remapped to http",
			dsn:         "clickhouse://default:@localhost:9000/default",
			wantBaseURL: "http://localhost:8123/",
			wantDB:      "default",
			wantUser:    "default",
			wantPass:    "",
		},
		{
			name:    "invalid scheme",
			dsn:     "tcp://localhost:9000/default",
			wantErr: true,
		},
		{
			name:    "missing host",
			dsn:     "http:///default",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseURL, db, user, pass, err := parseHTTPDSN(tt.dsn)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if baseURL != tt.wantBaseURL {
				t.Errorf("baseURL = %q, want %q", baseURL, tt.wantBaseURL)
			}
			if db != tt.wantDB {
				t.Errorf("db = %q, want %q", db, tt.wantDB)
			}
			if user != tt.wantUser {
				t.Errorf("user = %q, want %q", user, tt.wantUser)
			}
			if pass != tt.wantPass {
				t.Errorf("pass = %q, want %q", pass, tt.wantPass)
			}
		})
	}
}

func TestClickHouseWriter_WriteProducts(t *testing.T) {
	var queries []string
	var bodies []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		bodies = append(bodies, string(payload))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	dsn := ts.URL + "/default"
	writer, err := NewClickHouseWriter(context.Background(), dsn, zerolog.Nop())
	if err != nil {
		t.Fatalf("creating writer: %v", err)
	}

	products := []model.Product{
		{ID: 1, Price: 19.99, Category: "electronics"},
		{ID: 2, Price: 12.50, Category: ""},
	}
	count, err := writer.WriteProducts(context.Background(), products)
	if err != nil {
		t.Fatalf("WriteProducts: %v", err)
	}

	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if len(queries) != 2 {
		t.Fatalf("expected 2 queries (create + insert), got %d", len(queries))
	}
	if !strings.Contains(queries[0], "CREATE TABLE IF NOT EXISTS product_prices") {
		t.Errorf("unexpected create query: %q", queries[0])
	}
	if !strings.Contains(queries[1], "INSERT INTO product_prices") {
		t.Errorf("unexpected insert query: %q", queries[1])
	}
	if !strings.Contains(bodies[1], "\"category\":\"unknown\"") {
		t.Errorf("expected normalized category in insert payload, got: %q", bodies[1])
	}
}

func TestIsValidIdentifier(t *testing.T) {
	if !isValidIdentifier("product_prices") {
		t.Fatal("expected valid identifier")
	}
	if isValidIdentifier("bad-name") {
		t.Fatal("expected invalid identifier")
	}
	if isValidIdentifier("42table") {
		t.Fatal("expected invalid identifier")
	}
}
