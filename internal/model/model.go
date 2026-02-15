// Package model defines the data structures used by the collector.
package model

// Product represents a single product from the API.
type Product struct {
	ID       int     `json:"id"`
	Title    string  `json:"title"`
	Brand    string  `json:"brand"`
	Category string  `json:"category"`
	Price    float64 `json:"price"`
	Rating   float64 `json:"rating"`
	Stock    int     `json:"stock"`
}

// APIResponse is the top-level envelope returned by dummyjson.com.
type APIResponse struct {
	Products []Product `json:"products"`
	Total    int       `json:"total"`
	Skip     int       `json:"skip"`
	Limit    int       `json:"limit"`
}
