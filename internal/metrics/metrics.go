// Package metrics provides Prometheus instrumentation for the collector.
// It uses an explicit registry (not default global) to avoid test collisions.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for the collector.
type Metrics struct {
	// Store counters
	ProductsInserted prometheus.Counter
	ProductsUpdated  prometheus.Counter
	ProductsSkipped  prometheus.Counter

	// Fetch counters / histograms
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRetries         prometheus.Counter

	Registry *prometheus.Registry
}

// New creates and registers all collector metrics on a dedicated registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		ProductsInserted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "collector",
			Name:      "products_inserted_total",
			Help:      "Total number of products inserted into PostgreSQL.",
		}),
		ProductsUpdated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "collector",
			Name:      "products_updated_total",
			Help:      "Total number of products updated in PostgreSQL.",
		}),
		ProductsSkipped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "collector",
			Name:      "products_skipped_total",
			Help:      "Total number of products skipped (unchanged or invalid).",
		}),
		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "collector",
			Name:      "http_request_duration_seconds",
			Help:      "Duration of HTTP requests to the upstream API.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"status"}),
		HTTPRetries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "collector",
			Name:      "http_retries_total",
			Help:      "Total number of HTTP request retries.",
		}),
		Registry: reg,
	}

	reg.MustRegister(
		m.ProductsInserted,
		m.ProductsUpdated,
		m.ProductsSkipped,
		m.HTTPRequestDuration,
		m.HTTPRetries,
	)

	return m
}

// Handler returns an http.Handler that serves the /metrics endpoint
// using the dedicated registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
