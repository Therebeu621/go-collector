// Package fetch retrieves product data from a remote JSON API.
package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/anisse/collector/internal/model"
)

const defaultBaseURL = "https://dummyjson.com/products"

// FetchProducts performs a GET request and returns the parsed products.
// It uses a 10-second timeout to avoid hanging indefinitely.
func FetchProducts(ctx context.Context, baseURL string, limit int) ([]model.Product, error) {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	url := fmt.Sprintf("%s?limit=%d", baseURL, limit)

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	var apiResp model.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding JSON response: %w", err)
	}

	return apiResp.Products, nil
}
