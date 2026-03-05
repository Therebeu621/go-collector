package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"time"
)

const (
	maxRetries   = 3
	baseDelay    = 500 * time.Millisecond
	maxDelay     = 10 * time.Second
	jitterFactor = 0.25 // ±25%
)

// doWithRetry executes an HTTP request with exponential backoff and jitter.
// It retries only on transient errors: 429, 503, timeouts, and network errors.
// Non-transient 4xx errors (except 429) are never retried.
func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := backoffWithJitter(attempt)
			c.logger.Warn().
				Int("attempt", attempt).
				Dur("delay", delay).
				Str("url", req.URL.String()).
				Msg("retrying request")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.client.Do(req)
		if err != nil {
			if isRetriableError(err) {
				lastErr = err
				if c.metrics != nil {
					c.metrics.HTTPRetries.Inc()
				}
				continue
			}
			return nil, fmt.Errorf("executing %s %s: %w", req.Method, req.URL, err)
		}

		// Success
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		// Must drain and close body before retry to avoid leaking connections.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if isRetriableStatus(resp.StatusCode) {
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, req.URL)
			if c.metrics != nil {
				c.metrics.HTTPRetries.Inc()
			}
			continue
		}

		// Non-retriable HTTP error (4xx except 429)
		return nil, fmt.Errorf("non-retriable HTTP %d from %s", resp.StatusCode, req.URL)
	}

	return nil, fmt.Errorf("max retries exceeded for %s: %w", req.URL, lastErr)
}

// backoffWithJitter calculates an exponential delay with ±25% random jitter.
func backoffWithJitter(attempt int) time.Duration {
	delay := float64(baseDelay) * math.Pow(2, float64(attempt-1))
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}

	jitter := delay * jitterFactor * (2*rand.Float64() - 1) // [-25%, +25%]
	return time.Duration(delay + jitter)
}

// isRetriableStatus returns true for HTTP status codes that are transient.
func isRetriableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code == http.StatusServiceUnavailable
}

// isRetriableError returns true for network-level errors that are transient.
func isRetriableError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	return false
}
