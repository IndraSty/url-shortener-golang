package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/IndraSty/url-shortener-golang/config"
	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/geoip"
	"golang.org/x/crypto/bcrypt"
)

type redirectUsecase struct {
	linkRepo   domain.LinkRepository
	cacheRepo  domain.CacheRepository
	geoClient  *geoip.Client
	qstashCfg  config.QStashConfig
	baseURL    string
	httpClient *http.Client
}

// NewRedirectUsecase creates the redirect usecase — the performance-critical path.
func NewRedirectUsecase(
	linkRepo domain.LinkRepository,
	cacheRepo domain.CacheRepository,
	geoClient *geoip.Client,
	cfg *config.Config,
) domain.RedirectUsecase {
	return &redirectUsecase{
		linkRepo:  linkRepo,
		cacheRepo: cacheRepo,
		geoClient: geoClient,
		qstashCfg: cfg.QStash,
		baseURL:   cfg.App.BaseURL,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// Redirect is the hot path — target sub-10ms p99 latency.
//
// Order of operations (strict — do not reorder):
//  1. Redis cache lookup
//  2. On miss: PostgreSQL lookup → cache the result
//  3. Expiry check
//  4. Active check
//  5. Password check
//  6. Geo rule matching
//  7. A/B test weighted selection
//  8. Return destination + status code
func (u *redirectUsecase) Redirect(ctx context.Context, input domain.RedirectInput) (*domain.RedirectResult, error) {
	// -------------------------------------------------------------------------
	// Step 1 & 2: Cache-first lookup
	// -------------------------------------------------------------------------
	link, err := u.resolveLink(ctx, input.Slug)
	if err != nil {
		return nil, err
	}

	// -------------------------------------------------------------------------
	// Step 3: Expiry check
	// Return ErrLinkNotFound (same as missing) — prevents enumeration
	// -------------------------------------------------------------------------
	if link.IsExpired() {
		return nil, domain.ErrLinkNotFound
	}

	// -------------------------------------------------------------------------
	// Step 4: Active check
	// -------------------------------------------------------------------------
	if !link.IsActive {
		return nil, domain.ErrLinkNotFound
	}

	// -------------------------------------------------------------------------
	// Step 5: Password check
	// -------------------------------------------------------------------------
	if link.IsPasswordProtected() {
		if input.Password == "" {
			return nil, domain.ErrPasswordRequired
		}
		if err := bcrypt.CompareHashAndPassword([]byte(link.PasswordHash), []byte(input.Password)); err != nil {
			return nil, domain.ErrInvalidPassword
		}
	}

	// -------------------------------------------------------------------------
	// Step 6: Geo rule matching
	// Only perform geo lookup if the link has geo rules configured
	// -------------------------------------------------------------------------
	if len(link.GeoRules) > 0 {
		countryCode := u.resolveCountry(ctx, input.IP)
		if dest := matchGeoRule(link.GeoRules, countryCode); dest != "" {
			return &domain.RedirectResult{
				DestinationURL: dest,
				StatusCode:     http.StatusFound, // 302 — geo redirect may change
				LinkID:         link.ID,
			}, nil
		}
	}

	// -------------------------------------------------------------------------
	// Step 7: A/B test weighted selection
	// Only run if A/B variants are configured
	// -------------------------------------------------------------------------
	var abTestID string
	destination := link.DestinationURL
	statusCode := http.StatusMovedPermanently // 301 — permanent for non-A/B, non-geo

	if len(link.ABTests) > 0 {
		variant := selectABVariant(link.ABTests)
		if variant != nil {
			destination = variant.DestinationURL
			abTestID = variant.ID
			statusCode = http.StatusFound // 302 — A/B destinations may change
		}
	}

	return &domain.RedirectResult{
		DestinationURL: destination,
		StatusCode:     statusCode,
		ABTestID:       abTestID,
		LinkID:         link.ID,
	}, nil
}

func (u *redirectUsecase) UnlockWithPassword(ctx context.Context, slug string, password string) (*domain.RedirectResult, error) {
	return u.Redirect(ctx, domain.RedirectInput{
		Slug:     slug,
		Password: password,
	})
}

// PublishClickEvent sends a click event to QStash asynchronously.
// This is called AFTER the redirect response is sent — never blocks the redirect.
// If QStash is not configured (e.g. local dev), the event is silently dropped.
func (u *redirectUsecase) PublishClickEvent(ctx context.Context, payload domain.ClickEventPayload) error {
	if u.qstashCfg.Token == "" || u.qstashCfg.URL == "" {
		// QStash not configured — skip silently in development
		return nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("redirectUsecase.PublishClickEvent marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://qstash.upstash.io/v2/publish/%s", u.qstashCfg.URL),
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("redirectUsecase.PublishClickEvent create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+u.qstashCfg.Token)
	// QStash retry policy — retry up to 3 times with exponential backoff
	req.Header.Set("Upstash-Retries", "3")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		// Non-fatal — analytics loss is acceptable, redirect already sent
		return fmt.Errorf("redirectUsecase.PublishClickEvent http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("redirectUsecase.PublishClickEvent qstash status: %d", resp.StatusCode)
	}

	return nil
}

// --- Internal helpers ---

// resolveLink implements the cache-first lookup pattern.
// Redis → hit: return cached link
// Redis → miss: fetch from PostgreSQL → cache → return
func (u *redirectUsecase) resolveLink(ctx context.Context, slug string) (*domain.Link, error) {
	// Try Redis first
	cached, err := u.cacheRepo.GetLink(ctx, slug)
	if err != nil {
		// Cache error — fall through to DB, don't fail the redirect
		_ = err
	}
	if cached != nil {
		return cached, nil // cache hit
	}

	// Cache miss — fetch from PostgreSQL
	link, err := u.linkRepo.FindBySlug(ctx, slug)
	if err != nil {
		return nil, err // ErrLinkNotFound propagates
	}

	// Store in Redis for future requests — fire and forget
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		u.cacheRepo.SetLink(cacheCtx, link) //nolint:errcheck
	}()

	return link, nil
}

// resolveCountry returns the ISO country code for an IP address.
// Returns empty string on failure — geo rules simply won't match.
func (u *redirectUsecase) resolveCountry(ctx context.Context, ip string) string {
	if ip == "" {
		return ""
	}
	loc := u.geoClient.Lookup(ctx, ip)
	return loc.CountryCode
}

// matchGeoRule finds the first matching geo rule for a country code.
// Rules are already sorted by priority ASC from the repository.
// Returns empty string if no rule matches.
func matchGeoRule(rules []*domain.GeoRule, countryCode string) string {
	if countryCode == "" {
		return ""
	}
	for _, rule := range rules {
		if rule.CountryCode == countryCode {
			return rule.DestinationURL
		}
	}
	return ""
}

// selectABVariant picks a variant using weighted random selection.
// Algorithm: pick a random number in [0, totalWeight), walk variants until cumulative weight exceeds it.
//
// Example with weights [30, 20, 50]:
//   - rand in [0,30)  → variant 0
//   - rand in [30,50) → variant 1
//   - rand in [50,100)→ variant 2
//
// This is O(n) but n is always tiny (max ~10 variants).
func selectABVariant(variants []*domain.ABTest) *domain.ABTest {
	if len(variants) == 0 {
		return nil
	}

	// Sum total weight — may not be exactly 100 if variants are being added incrementally
	totalWeight := 0
	for _, v := range variants {
		totalWeight += v.Weight
	}
	if totalWeight <= 0 {
		return variants[0] // fallback
	}

	// Pick a random point in [0, totalWeight)
	pick := rand.Intn(totalWeight) //nolint:gosec — non-cryptographic random is correct here

	cumulative := 0
	for _, v := range variants {
		cumulative += v.Weight
		if pick < cumulative {
			return v
		}
	}

	// Should never reach here, but return last variant as safety fallback
	return variants[len(variants)-1]
}
