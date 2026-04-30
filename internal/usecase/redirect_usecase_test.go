package usecase_test

import (
	"testing"
	"time"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/internal/usecase"
	"github.com/stretchr/testify/assert"
)

// Alias the exported test helpers for cleaner test code
var (
	selectVariant = usecase.SelectVariantForTest
	matchGeo      = usecase.MatchGeoRuleForTest
)

// ---------------------------------------------------------------------------
// A/B test weighted selection
// ---------------------------------------------------------------------------

// selectABVariant is exported for testing via a thin wrapper.
// We test the algorithm directly without needing the full usecase.

func TestSelectABVariant_SingleVariant(t *testing.T) {
	variants := []*domain.ABTest{
		{ID: "a", Weight: 100, DestinationURL: "https://example.com/a"},
	}

	// With only one variant at 100 weight, it must always win
	for i := 0; i < 100; i++ {
		got := selectVariant(variants)
		assert.Equal(t, "a", got.ID)
	}
}

func TestSelectABVariant_EqualWeights(t *testing.T) {
	variants := []*domain.ABTest{
		{ID: "a", Weight: 50, DestinationURL: "https://example.com/a"},
		{ID: "b", Weight: 50, DestinationURL: "https://example.com/b"},
	}

	counts := map[string]int{"a": 0, "b": 0}

	// Run 10000 selections — distribution should be roughly 50/50
	const iterations = 10000
	for i := 0; i < iterations; i++ {
		got := selectVariant(variants)
		counts[got.ID]++
	}

	// Allow 5% tolerance — expect 4500–5500 each
	assert.InDelta(t, 5000, counts["a"], 500, "variant A distribution out of range")
	assert.InDelta(t, 5000, counts["b"], 500, "variant B distribution out of range")
}

func TestSelectABVariant_WeightedDistribution(t *testing.T) {
	variants := []*domain.ABTest{
		{ID: "control", Weight: 80, DestinationURL: "https://example.com/control"},
		{ID: "variant", Weight: 20, DestinationURL: "https://example.com/variant"},
	}

	counts := map[string]int{"control": 0, "variant": 0}

	const iterations = 10000
	for i := 0; i < iterations; i++ {
		got := selectVariant(variants)
		counts[got.ID]++
	}

	// Control should get ~80%, variant ~20% — allow 3% tolerance
	assert.InDelta(t, 8000, counts["control"], 300)
	assert.InDelta(t, 2000, counts["variant"], 300)
}

func TestSelectABVariant_ThreeWaySplit(t *testing.T) {
	variants := []*domain.ABTest{
		{ID: "a", Weight: 33, DestinationURL: "https://a.com"},
		{ID: "b", Weight: 33, DestinationURL: "https://b.com"},
		{ID: "c", Weight: 34, DestinationURL: "https://c.com"},
	}

	counts := map[string]int{"a": 0, "b": 0, "c": 0}

	const iterations = 10000
	for i := 0; i < iterations; i++ {
		got := selectVariant(variants)
		counts[got.ID]++
	}

	assert.InDelta(t, 3300, counts["a"], 300)
	assert.InDelta(t, 3300, counts["b"], 300)
	assert.InDelta(t, 3400, counts["c"], 300)
}

func TestSelectABVariant_Empty(t *testing.T) {
	got := selectVariant([]*domain.ABTest{})
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// Geo rule matching
// ---------------------------------------------------------------------------

func TestMatchGeoRule_Hit(t *testing.T) {
	rules := []*domain.GeoRule{
		{CountryCode: "US", DestinationURL: "https://us.example.com"},
		{CountryCode: "ID", DestinationURL: "https://id.example.com"},
		{CountryCode: "SG", DestinationURL: "https://sg.example.com"},
	}

	assert.Equal(t, "https://id.example.com", matchGeo(rules, "ID"))
	assert.Equal(t, "https://us.example.com", matchGeo(rules, "US"))
	assert.Equal(t, "https://sg.example.com", matchGeo(rules, "SG"))
}

func TestMatchGeoRule_Miss(t *testing.T) {
	rules := []*domain.GeoRule{
		{CountryCode: "US", DestinationURL: "https://us.example.com"},
	}

	// No rule for Japan — should return empty string (fall through to A/B or default)
	assert.Equal(t, "", matchGeo(rules, "JP"))
}

func TestMatchGeoRule_EmptyCountry(t *testing.T) {
	rules := []*domain.GeoRule{
		{CountryCode: "US", DestinationURL: "https://us.example.com"},
	}

	// Empty country code (geo lookup failed) — should not match anything
	assert.Equal(t, "", matchGeo(rules, ""))
}

func TestMatchGeoRule_NoRules(t *testing.T) {
	assert.Equal(t, "", matchGeo([]*domain.GeoRule{}, "ID"))
}

func TestMatchGeoRule_Priority(t *testing.T) {
	// Rules are pre-sorted by priority ASC from the repository
	// First match wins — lower priority number = evaluated first
	rules := []*domain.GeoRule{
		{CountryCode: "US", DestinationURL: "https://first.com", Priority: 0},
		{CountryCode: "US", DestinationURL: "https://second.com", Priority: 1},
	}

	// Should return the first match (priority 0)
	assert.Equal(t, "https://first.com", matchGeo(rules, "US"))
}

// ---------------------------------------------------------------------------
// Link expiry
// ---------------------------------------------------------------------------

func TestLinkIsExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	expiredLink := &domain.Link{ExpiredAt: &past}
	activeLink := &domain.Link{ExpiredAt: &future}
	noExpiryLink := &domain.Link{ExpiredAt: nil}

	assert.True(t, expiredLink.IsExpired())
	assert.False(t, activeLink.IsExpired())
	assert.False(t, noExpiryLink.IsExpired())
}

// ---------------------------------------------------------------------------
// Helpers — thin wrappers to call unexported functions via same package
// We put these tests in package usecase_test so we need exported wrappers.
// Instead, we use a test export file.
// ---------------------------------------------------------------------------
