package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/metrics"
	"github.com/redis/go-redis/v9"
)

const (
	// linkKeyPrefix is the Redis key prefix for cached links.
	// Format: link:{slug}
	linkKeyPrefix = "link:"

	// defaultLinkTTL is how long we cache a link when it has no expiry set.
	// 24 hours balances freshness with cache hit rate.
	defaultLinkTTL = 24 * time.Hour

	// rateLimitKeyPrefix is the prefix for rate limit counters.
	// Format: rl:{identifier}
	rateLimitKeyPrefix = "rl:"
)

// cachedLink is the Redis-serialized form of a domain.Link.
// We use a dedicated struct instead of serializing domain.Link directly
// to avoid coupling the cache format to the domain struct layout.
type cachedLink struct {
	ID             int64           `json:"id"`
	UserID         string          `json:"user_id"`
	Slug           string          `json:"slug"`
	DestinationURL string          `json:"destination_url"`
	Title          string          `json:"title"`
	PasswordHash   string          `json:"password_hash"`
	IsActive       bool            `json:"is_active"`
	ClickCount     int64           `json:"click_count"`
	ExpiredAt      *time.Time      `json:"expired_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	ABTests        []cachedABTest  `json:"ab_tests"`
	GeoRules       []cachedGeoRule `json:"geo_rules"`
}

type cachedABTest struct {
	ID             string    `json:"id"`
	LinkID         int64     `json:"link_id"`
	DestinationURL string    `json:"destination_url"`
	Weight         int       `json:"weight"`
	Label          string    `json:"label"`
	CreatedAt      time.Time `json:"created_at"`
}

type cachedGeoRule struct {
	ID             string    `json:"id"`
	LinkID         int64     `json:"link_id"`
	CountryCode    string    `json:"country_code"`
	DestinationURL string    `json:"destination_url"`
	Priority       int       `json:"priority"`
	CreatedAt      time.Time `json:"created_at"`
}

type cacheRepository struct {
	client *redis.Client
}

// NewCacheRepository creates a Redis-backed cache repository.
func NewCacheRepository(client *redis.Client) domain.CacheRepository {
	return &cacheRepository{client: client}
}

func (r *cacheRepository) GetLink(ctx context.Context, slug string) (*domain.Link, error) {
	key := linkKeyPrefix + slug

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		// Cache miss is not an error — caller falls back to PostgreSQL
		if errors.Is(err, redis.Nil) {
			metrics.CacheMissesTotal.Inc()
			return nil, nil
		}
		return nil, fmt.Errorf("cacheRepository.GetLink: %w", err)
	}

	var cached cachedLink
	if err := json.Unmarshal(data, &cached); err != nil {
		// Corrupted cache entry — treat as miss, let it get refreshed
		metrics.CacheMissesTotal.Inc()
		return nil, nil
	}

	metrics.CacheHitsTotal.Inc()
	return toDomainLink(cached), nil
}

func (r *cacheRepository) SetLink(ctx context.Context, link *domain.Link) error {
	key := linkKeyPrefix + link.Slug

	cached := fromDomainLink(link)
	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("cacheRepository.SetLink marshal: %w", err)
	}

	// Calculate TTL based on link expiry
	ttl := defaultLinkTTL
	if link.ExpiredAt != nil {
		remaining := time.Until(*link.ExpiredAt)
		if remaining <= 0 {
			// Already expired — don't cache it
			return nil
		}
		// Cache until expiry + small buffer
		ttl = remaining + 10*time.Second
	}

	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("cacheRepository.SetLink: %w", err)
	}

	return nil
}

func (r *cacheRepository) DeleteLink(ctx context.Context, slug string) error {
	key := linkKeyPrefix + slug
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("cacheRepository.DeleteLink: %w", err)
	}
	return nil
}

// IncrRateLimit implements a Redis sliding window rate limiter using
// a simple counter with expiry reset strategy (fixed window approximation).
// For the redirect endpoint, this is accurate enough and extremely fast.
//
// Returns (currentCount, limitExceeded, error).
func (r *cacheRepository) IncrRateLimit(ctx context.Context, key string, windowSecs int, limit int) (int64, bool, error) {
	fullKey := rateLimitKeyPrefix + key

	// Use a pipeline to INCR and set expiry atomically
	pipe := r.client.Pipeline()
	incrCmd := pipe.Incr(ctx, fullKey)
	pipe.Expire(ctx, fullKey, time.Duration(windowSecs)*time.Second)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, false, fmt.Errorf("cacheRepository.IncrRateLimit: %w", err)
	}

	count := incrCmd.Val()
	return count, count > int64(limit), nil
}

// --- Conversion helpers ---

func fromDomainLink(l *domain.Link) cachedLink {
	c := cachedLink{
		ID:             l.ID,
		UserID:         l.UserID,
		Slug:           l.Slug,
		DestinationURL: l.DestinationURL,
		Title:          l.Title,
		PasswordHash:   l.PasswordHash,
		IsActive:       l.IsActive,
		ClickCount:     l.ClickCount,
		ExpiredAt:      l.ExpiredAt,
		CreatedAt:      l.CreatedAt,
		UpdatedAt:      l.UpdatedAt,
	}

	for _, ab := range l.ABTests {
		c.ABTests = append(c.ABTests, cachedABTest{
			ID: ab.ID, LinkID: ab.LinkID, DestinationURL: ab.DestinationURL,
			Weight: ab.Weight, Label: ab.Label, CreatedAt: ab.CreatedAt,
		})
	}

	for _, gr := range l.GeoRules {
		c.GeoRules = append(c.GeoRules, cachedGeoRule{
			ID: gr.ID, LinkID: gr.LinkID, CountryCode: gr.CountryCode,
			DestinationURL: gr.DestinationURL, Priority: gr.Priority, CreatedAt: gr.CreatedAt,
		})
	}

	return c
}

func toDomainLink(c cachedLink) *domain.Link {
	l := &domain.Link{
		ID:             c.ID,
		UserID:         c.UserID,
		Slug:           c.Slug,
		DestinationURL: c.DestinationURL,
		Title:          c.Title,
		PasswordHash:   c.PasswordHash,
		IsActive:       c.IsActive,
		ClickCount:     c.ClickCount,
		ExpiredAt:      c.ExpiredAt,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}

	for _, ab := range c.ABTests {
		l.ABTests = append(l.ABTests, &domain.ABTest{
			ID: ab.ID, LinkID: ab.LinkID, DestinationURL: ab.DestinationURL,
			Weight: ab.Weight, Label: ab.Label, CreatedAt: ab.CreatedAt,
		})
	}

	for _, gr := range c.GeoRules {
		l.GeoRules = append(l.GeoRules, &domain.GeoRule{
			ID: gr.ID, LinkID: gr.LinkID, CountryCode: gr.CountryCode,
			DestinationURL: gr.DestinationURL, Priority: gr.Priority, CreatedAt: gr.CreatedAt,
		})
	}

	return l
}
