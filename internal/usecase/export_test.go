//go:build !production

package usecase

import "github.com/IndraSty/url-shortener-golang/internal/domain"

// Exported wrappers for testing unexported functions.
// This file is only compiled during go test — never in production builds.

// SelectVariantForTest exposes selectABVariant for unit testing.
func SelectVariantForTest(variants []*domain.ABTest) *domain.ABTest {
	return selectABVariant(variants)
}

// MatchGeoRuleForTest exposes matchGeoRule for unit testing.
func MatchGeoRuleForTest(rules []*domain.GeoRule, countryCode string) string {
	return matchGeoRule(rules, countryCode)
}
