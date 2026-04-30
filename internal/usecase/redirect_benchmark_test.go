package usecase_test

import (
	"testing"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/internal/usecase"
)

// ---------------------------------------------------------------------------
// Benchmark: A/B variant selection
// Goal: prove selection is sub-microsecond — negligible redirect overhead
// ---------------------------------------------------------------------------

func BenchmarkSelectABVariant_TwoVariants(b *testing.B) {
	variants := []*domain.ABTest{
		{ID: "a", Weight: 70, DestinationURL: "https://example.com/a"},
		{ID: "b", Weight: 30, DestinationURL: "https://example.com/b"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		usecase.SelectVariantForTest(variants)
	}
}

func BenchmarkSelectABVariant_TenVariants(b *testing.B) {
	variants := []*domain.ABTest{
		{ID: "a", Weight: 10, DestinationURL: "https://example.com/a"},
		{ID: "b", Weight: 10, DestinationURL: "https://example.com/b"},
		{ID: "c", Weight: 10, DestinationURL: "https://example.com/c"},
		{ID: "d", Weight: 10, DestinationURL: "https://example.com/d"},
		{ID: "e", Weight: 10, DestinationURL: "https://example.com/e"},
		{ID: "f", Weight: 10, DestinationURL: "https://example.com/f"},
		{ID: "g", Weight: 10, DestinationURL: "https://example.com/g"},
		{ID: "h", Weight: 10, DestinationURL: "https://example.com/h"},
		{ID: "i", Weight: 10, DestinationURL: "https://example.com/i"},
		{ID: "j", Weight: 10, DestinationURL: "https://example.com/j"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		usecase.SelectVariantForTest(variants)
	}
}

func BenchmarkMatchGeoRule(b *testing.B) {
	rules := []*domain.GeoRule{
		{CountryCode: "US", DestinationURL: "https://us.example.com", Priority: 0},
		{CountryCode: "SG", DestinationURL: "https://sg.example.com", Priority: 1},
		{CountryCode: "ID", DestinationURL: "https://id.example.com", Priority: 2},
		{CountryCode: "JP", DestinationURL: "https://jp.example.com", Priority: 3},
		{CountryCode: "AU", DestinationURL: "https://au.example.com", Priority: 4},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		usecase.MatchGeoRuleForTest(rules, "ID")
	}
}

// ---------------------------------------------------------------------------
// Benchmark: Cache-first redirect resolution (mock)
// This benchmark uses a mock cache to measure pure business logic latency
// without network I/O — proves the logic itself is sub-millisecond.
// ---------------------------------------------------------------------------

func BenchmarkRedirectDecisionLogic(b *testing.B) {
	// Simulate a fully cached link with A/B test and geo rules
	link := &domain.Link{
		ID:             1,
		Slug:           "bench",
		DestinationURL: "https://default.example.com",
		IsActive:       true,
		ABTests: []*domain.ABTest{
			{ID: "a", Weight: 60, DestinationURL: "https://example.com/a"},
			{ID: "b", Weight: 40, DestinationURL: "https://example.com/b"},
		},
		GeoRules: []*domain.GeoRule{
			{CountryCode: "ID", DestinationURL: "https://id.example.com", Priority: 0},
			{CountryCode: "US", DestinationURL: "https://us.example.com", Priority: 1},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate the decision logic without cache/DB I/O:
		// 1. Expiry check
		if link.IsExpired() {
			continue
		}

		// 2. Active check
		if !link.IsActive {
			continue
		}

		// 3. Geo match
		if dest := usecase.MatchGeoRuleForTest(link.GeoRules, "ID"); dest != "" {
			_ = dest
			continue
		}

		// 4. A/B selection
		if variant := usecase.SelectVariantForTest(link.ABTests); variant != nil {
			_ = variant.DestinationURL
		}
	}
}
