// Package fetch retrieves product data from a remote JSON API with
// concurrent pagination, rate limiting, and retry logic.
package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/anisse/collector/internal/metrics"
	"github.com/anisse/collector/internal/model"
)

// Fetcher is the interface for product data retrieval.
type Fetcher interface {
	FetchAll(ctx context.Context) ([]model.Product, error)
}

// pageJob describes a single page to fetch.
type pageJob struct {
	skip  int
	limit int
}

// Client implements Fetcher with pagination, a worker pool, rate limiting,
// and retry with exponential backoff.
type Client struct {
	baseURL     string
	maxProducts int // total cap on products to return (LIMIT)
	pageSize    int // products per API page (PAGE_SIZE)
	workers     int
	client      *http.Client
	limiter     *rate.Limiter
	logger      zerolog.Logger
	metrics     *metrics.Metrics // optional, may be nil
}

// NewClient creates a fetch Client. The http.Client should be pre-configured
// with timeouts and proxy settings by the caller.
// maxProducts is the total number of products to return (LIMIT).
// pageSize is the number of products per API page (PAGE_SIZE).
func NewClient(baseURL string, maxProducts, pageSize, workers, ratePerSec int, httpClient *http.Client, logger zerolog.Logger, m *metrics.Metrics) *Client {
	return &Client{
		baseURL:     baseURL,
		maxProducts: maxProducts,
		pageSize:    pageSize,
		workers:     workers,
		client:      httpClient,
		limiter:     rate.NewLimiter(rate.Limit(ratePerSec), 1),
		logger:      logger,
		metrics:     m,
	}
}

// FetchAll discovers the total product count, fans out page jobs to workers,
// and returns the merged list of products sorted by ID, capped at maxProducts.
func (c *Client) FetchAll(ctx context.Context) ([]model.Product, error) {
	// 1. Discovery: fetch the first page to learn the total count.
	firstPage, apiTotal, err := c.fetchPage(ctx, 0, c.pageSize)
	if err != nil {
		return nil, fmt.Errorf("discovery request: %w", err)
	}

	// Cap at maxProducts if the API has more.
	total := apiTotal
	if c.maxProducts > 0 && total > c.maxProducts {
		total = c.maxProducts
	}

	c.logger.Info().
		Int("api_total", apiTotal).
		Int("limit", c.maxProducts).
		Int("effective_total", total).
		Int("page_size", c.pageSize).
		Int("workers", c.workers).
		Msg("discovered total products, starting pagination")

	if total <= c.pageSize {
		// Everything fits in the first page.
		return firstPage, nil
	}

	// 2. Build remaining page jobs (only up to 'total' capped products).
	var jobs []pageJob
	for skip := c.pageSize; skip < total; skip += c.pageSize {
		jobs = append(jobs, pageJob{skip: skip, limit: c.pageSize})
	}

	// 3. Fan out to workers via errgroup.
	var mu sync.Mutex
	allProducts := make([]model.Product, 0, total)
	allProducts = append(allProducts, firstPage...)

	g, gCtx := errgroup.WithContext(ctx)
	jobCh := make(chan pageJob, len(jobs))

	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	for i := 0; i < c.workers; i++ {
		g.Go(func() error {
			for job := range jobCh {
				products, _, err := c.fetchPage(gCtx, job.skip, job.limit)
				if err != nil {
					return fmt.Errorf("page skip=%d: %w", job.skip, err)
				}

				mu.Lock()
				allProducts = append(allProducts, products...)
				mu.Unlock()

				c.logger.Debug().
					Int("skip", job.skip).
					Int("fetched", len(products)).
					Msg("page fetched")
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// 4. Sort by ID for deterministic output.
	sort.Slice(allProducts, func(i, j int) bool {
		return allProducts[i].ID < allProducts[j].ID
	})

	// 5. Cap at maxProducts.
	if c.maxProducts > 0 && len(allProducts) > c.maxProducts {
		allProducts = allProducts[:c.maxProducts]
	}

	c.logger.Info().
		Int("total_fetched", len(allProducts)).
		Msg("all pages fetched")

	return allProducts, nil
}

// fetchPage performs a single paginated GET, with rate limiting and retry.
func (c *Client) fetchPage(ctx context.Context, skip, limit int) ([]model.Product, int, error) {
	url := fmt.Sprintf("%s?limit=%d&skip=%d", c.baseURL, limit, skip)

	// Rate limit: wait for token before making the request.
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, 0, fmt.Errorf("rate limiter: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}

	start := time.Now()

	resp, err := c.doWithRetry(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// Record HTTP request duration.
	if c.metrics != nil {
		c.metrics.HTTPRequestDuration.WithLabelValues(strconv.Itoa(resp.StatusCode)).Observe(time.Since(start).Seconds())
	}

	var apiResp model.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, 0, fmt.Errorf("decoding JSON from %s: %w", url, err)
	}

	return apiResp.Products, apiResp.Total, nil
}
