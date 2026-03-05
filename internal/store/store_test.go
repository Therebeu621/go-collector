package store

import (
	"testing"

	"github.com/anisse/collector/internal/model"
)

func TestValidateAndNormalize_ValidProduct(t *testing.T) {
	p := model.Product{
		ID:       1,
		Title:    "Laptop",
		Brand:    "BrandX",
		Category: "electronics",
		Price:    999.99,
		Rating:   4.5,
		Stock:    50,
	}

	result, reasons, ok := ValidateAndNormalize(p)
	if !ok {
		t.Fatal("expected product to be valid")
	}
	if len(reasons) != 0 {
		t.Errorf("expected no reasons, got %v", reasons)
	}
	if result.Category != "electronics" {
		t.Errorf("category = %q, want electronics", result.Category)
	}
}

func TestValidateAndNormalize_EmptyTitle(t *testing.T) {
	p := model.Product{
		ID:    1,
		Title: "",
		Price: 10,
	}

	_, reasons, ok := ValidateAndNormalize(p)
	if ok {
		t.Fatal("expected product to be rejected")
	}
	if len(reasons) == 0 || reasons[0] != "title is empty" {
		t.Errorf("unexpected reasons: %v", reasons)
	}
}

func TestValidateAndNormalize_NegativePrice(t *testing.T) {
	p := model.Product{
		ID:    1,
		Title: "Test",
		Price: -5.00,
	}

	_, _, ok := ValidateAndNormalize(p)
	if ok {
		t.Fatal("expected product to be rejected for negative price")
	}
}

func TestValidateAndNormalize_ZeroPrice(t *testing.T) {
	p := model.Product{
		ID:    1,
		Title: "Test",
		Price: 0,
	}

	_, _, ok := ValidateAndNormalize(p)
	if ok {
		t.Fatal("expected product to be rejected for zero price")
	}
}

func TestValidateAndNormalize_EmptyCategory(t *testing.T) {
	p := model.Product{
		ID:       1,
		Title:    "Test",
		Category: "",
		Price:    10,
	}

	result, reasons, ok := ValidateAndNormalize(p)
	if !ok {
		t.Fatal("expected product to be valid")
	}
	if result.Category != "unknown" {
		t.Errorf("category = %q, want unknown", result.Category)
	}
	if len(reasons) != 1 {
		t.Errorf("expected 1 reason, got %v", reasons)
	}
}

func TestValidateAndNormalize_HighPrice(t *testing.T) {
	p := model.Product{
		ID:       1,
		Title:    "Expensive",
		Category: "luxury",
		Price:    15000,
	}

	_, reasons, ok := ValidateAndNormalize(p)
	if !ok {
		t.Fatal("expected product to be valid (warn, not skip)")
	}
	if len(reasons) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(reasons), reasons)
	}
}

func TestValidateAndNormalize_NegativeStock(t *testing.T) {
	p := model.Product{
		ID:       1,
		Title:    "Test",
		Category: "test",
		Price:    10,
		Stock:    -3,
	}

	_, reasons, ok := ValidateAndNormalize(p)
	if !ok {
		t.Fatal("expected product to be valid (warn, not skip)")
	}
	if len(reasons) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(reasons), reasons)
	}
}

func TestValidateAndNormalize_MultipleWarnings(t *testing.T) {
	p := model.Product{
		ID:       1,
		Title:    "Test",
		Category: "", // will normalize to "unknown"
		Price:    15000,
		Stock:    -1,
	}

	result, reasons, ok := ValidateAndNormalize(p)
	if !ok {
		t.Fatal("expected product to be valid")
	}
	if result.Category != "unknown" {
		t.Errorf("category = %q, want unknown", result.Category)
	}
	// Expect 3 reasons: empty category + high price + negative stock
	if len(reasons) != 3 {
		t.Fatalf("expected 3 reasons, got %d: %v", len(reasons), reasons)
	}
}

func TestComputeChecksum_Deterministic(t *testing.T) {
	p := model.Product{
		ID:       1,
		Title:    "Test",
		Category: "test",
		Price:    10,
		Stock:    5,
	}

	c1 := computeChecksum(p)
	c2 := computeChecksum(p)

	if c1 != c2 {
		t.Errorf("checksum not deterministic: %q != %q", c1, c2)
	}

	if len(c1) != 64 { // SHA-256 hex = 64 chars
		t.Errorf("checksum length = %d, want 64", len(c1))
	}
}

func TestComputeChecksum_DifferentProducts(t *testing.T) {
	p1 := model.Product{ID: 1, Title: "A", Price: 10}
	p2 := model.Product{ID: 1, Title: "B", Price: 10}

	c1 := computeChecksum(p1)
	c2 := computeChecksum(p2)

	if c1 == c2 {
		t.Error("different products should have different checksums")
	}
}
