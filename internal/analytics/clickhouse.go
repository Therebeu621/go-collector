// Package analytics provides optional sinks for analytical databases.
package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/anisse/collector/internal/model"
)

const defaultClickHouseTable = "product_prices"

var identifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ClickHouseWriter writes product snapshots into ClickHouse over HTTP.
// It is optional and only used when CLICKHOUSE_DSN is configured.
type ClickHouseWriter struct {
	client   *http.Client
	baseURL  string
	database string
	username string
	password string
	table    string
	logger   zerolog.Logger
}

// NewClickHouseWriter creates a ClickHouse HTTP writer from a DSN.
//
// Supported DSN examples:
//   - http://default:@localhost:8123/default
//   - https://user:pass@clickhouse.my-company.net:8443/analytics
//   - clickhouse://default:@localhost/default (converted to http://localhost:8123/default)
func NewClickHouseWriter(ctx context.Context, dsn string, logger zerolog.Logger) (*ClickHouseWriter, error) {
	baseURL, database, username, password, err := parseHTTPDSN(dsn)
	if err != nil {
		return nil, err
	}

	w := &ClickHouseWriter{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:  baseURL,
		database: database,
		username: username,
		password: password,
		table:    defaultClickHouseTable,
		logger:   logger,
	}

	if err := w.ensureTable(ctx); err != nil {
		return nil, err
	}

	return w, nil
}

// Close is a no-op for the HTTP writer and is kept for symmetrical lifecycle handling.
func (w *ClickHouseWriter) Close() error {
	return nil
}

// WriteProducts inserts a JSONEachRow batch into ClickHouse.
// Returns the number of exported rows.
func (w *ClickHouseWriter) WriteProducts(ctx context.Context, products []model.Product) (int, error) {
	if len(products) == 0 {
		return 0, nil
	}

	type clickhouseRow struct {
		ID          uint32  `json:"id"`
		Price       float64 `json:"price"`
		Category    string  `json:"category"`
		CollectedAt int64   `json:"collected_at"`
	}

	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	collectedAt := time.Now().UTC().Unix()

	exported := 0
	for _, p := range products {
		if p.ID < 0 {
			w.logger.Warn().Int("product_id", p.ID).Msg("skipping negative product id for clickhouse export")
			continue
		}

		category := p.Category
		if category == "" {
			category = "unknown"
		}

		row := clickhouseRow{
			ID:          uint32(p.ID),
			Price:       p.Price,
			Category:    category,
			CollectedAt: collectedAt,
		}

		if err := encoder.Encode(row); err != nil {
			return exported, fmt.Errorf("encoding clickhouse row: %w", err)
		}
		exported++
	}

	if exported == 0 {
		return 0, nil
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (id, price, category, collected_at) FORMAT JSONEachRow",
		w.table,
	)
	if err := w.exec(ctx, query, &body); err != nil {
		return exported, err
	}

	return exported, nil
}

func (w *ClickHouseWriter) ensureTable(ctx context.Context) error {
	if !isValidIdentifier(w.table) {
		return fmt.Errorf("invalid clickhouse table name %q", w.table)
	}

	query := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    id UInt32,
    price Float64,
    category String,
    collected_at DateTime
) ENGINE = MergeTree()
ORDER BY (collected_at, id)
`, w.table)

	if err := w.exec(ctx, query, nil); err != nil {
		return fmt.Errorf("ensuring clickhouse table: %w", err)
	}

	return nil
}

func (w *ClickHouseWriter) exec(ctx context.Context, query string, body io.Reader) error {
	endpoint, err := url.Parse(w.baseURL)
	if err != nil {
		return fmt.Errorf("parsing clickhouse base URL: %w", err)
	}

	values := endpoint.Query()
	values.Set("database", w.database)
	values.Set("query", query)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("creating clickhouse request: %w", err)
	}
	if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing clickhouse request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("clickhouse HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("draining clickhouse response: %w", err)
	}

	return nil
}

func parseHTTPDSN(dsn string) (baseURL, database, username, password string, err error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid CLICKHOUSE_DSN: %w", err)
	}

	switch u.Scheme {
	case "http", "https":
	case "clickhouse":
		// Allow clickhouse://... but write through HTTP API.
		u.Scheme = "http"
		host := u.Hostname()
		port := u.Port()
		if port == "" || port == "9000" {
			port = "8123"
		}
		u.Host = host + ":" + port
	default:
		return "", "", "", "", fmt.Errorf("unsupported CLICKHOUSE_DSN scheme %q (expected http/https/clickhouse)", u.Scheme)
	}

	if u.Host == "" {
		return "", "", "", "", fmt.Errorf("invalid CLICKHOUSE_DSN: host is required")
	}

	database = strings.Trim(u.Path, "/")
	if database == "" {
		database = "default"
	}

	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}

	base := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   "/",
	}
	return base.String(), database, username, password, nil
}

func isValidIdentifier(name string) bool {
	return identifierPattern.MatchString(name)
}
