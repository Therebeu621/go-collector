package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/anisse/collector/internal/model"
)

// newTestLogger returns a no-op logger for tests.
func newTestLogger() zerolog.Logger {
	return zerolog.Nop()
}

// newTestServer creates an httptest server that serves paginated products.
// totalProducts is the total number of products to simulate.
func newTestServer(totalProducts int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := 10
		skip := 0
		if v := q.Get("limit"); v != "" {
			fmt.Sscanf(v, "%d", &limit)
		}
		if v := q.Get("skip"); v != "" {
			fmt.Sscanf(v, "%d", &skip)
		}

		var products []model.Product
		for i := skip; i < skip+limit && i < totalProducts; i++ {
			products = append(products, model.Product{
				ID:       i + 1,
				Title:    fmt.Sprintf("Product %d", i+1),
				Brand:    "TestBrand",
				Category: "test",
				Price:    float64(i+1) * 10,
				Rating:   4.5,
				Stock:    100,
			})
		}

		resp := model.APIResponse{
			Products: products,
			Total:    totalProducts,
			Skip:     skip,
			Limit:    limit,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestFetchAll_SinglePage(t *testing.T) {
	ts := newTestServer(5)
	defer ts.Close()

	// maxProducts=10 (want at most 10), pageSize=10, 2 workers
	client := NewClient(ts.URL, 10, 10, 2, 100, ts.Client(), newTestLogger())

	products, err := client.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(products) != 5 {
		t.Errorf("got %d products, want 5", len(products))
	}
}

func TestFetchAll_Pagination(t *testing.T) {
	ts := newTestServer(25)
	defer ts.Close()

	// maxProducts=25 (want all), pageSize=10, 3 workers
	client := NewClient(ts.URL, 25, 10, 3, 100, ts.Client(), newTestLogger())

	products, err := client.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(products) != 25 {
		t.Errorf("got %d products, want 25", len(products))
	}

	// Verify sorted by ID.
	for i := 1; i < len(products); i++ {
		if products[i].ID <= products[i-1].ID {
			t.Errorf("products not sorted: ID %d after ID %d", products[i].ID, products[i-1].ID)
		}
	}
}

func TestFetchAll_RetryOn429(t *testing.T) {
	var attempts atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		resp := model.APIResponse{
			Products: []model.Product{
				{ID: 1, Title: "Retry Product", Price: 10, Category: "test"},
			},
			Total: 1,
			Limit: 10,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, 10, 10, 1, 100, ts.Client(), newTestLogger())

	products, err := client.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll after retry: %v", err)
	}

	if len(products) != 1 {
		t.Errorf("got %d products, want 1", len(products))
	}

	if attempts.Load() < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts.Load())
	}
}

func TestFetchAll_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Slow server
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	httpClient := &http.Client{Timeout: 1 * time.Second}
	client := NewClient(ts.URL, 10, 10, 1, 100, httpClient, newTestLogger())

	_, err := client.FetchAll(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestFetchAll_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json at all{{{"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL, 10, 10, 1, 100, ts.Client(), newTestLogger())

	_, err := client.FetchAll(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestFetchAll_NonRetriableError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // 403 — not retriable
	}))
	defer ts.Close()

	client := NewClient(ts.URL, 10, 10, 1, 100, ts.Client(), newTestLogger())

	_, err := client.FetchAll(context.Background())
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestFetchAll_LimitCapsProducts(t *testing.T) {
	// API has 50 products, but LIMIT=15 should cap the result.
	ts := newTestServer(50)
	defer ts.Close()

	// maxProducts=15, pageSize=10 => fetches 2 pages but caps at 15.
	client := NewClient(ts.URL, 15, 10, 2, 100, ts.Client(), newTestLogger())

	products, err := client.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(products) != 15 {
		t.Errorf("got %d products, want 15 (LIMIT should cap)", len(products))
	}
}
