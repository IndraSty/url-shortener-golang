package domain

import "context"

// RedirectResult is returned by the redirect usecase.
// It carries everything the HTTP handler needs to issue the redirect response.
type RedirectResult struct {
	DestinationURL string
	StatusCode     int    // 301 (permanent) or 302 (temporary — used for A/B, geo, expiring links)
	ABTestID       string // populated when A/B variant was selected, for analytics
	LinkID         int64  // needed by redirect handler to build click event payload
}

// RedirectInput carries the raw data from the incoming redirect request.
type RedirectInput struct {
	Slug      string
	IP        string
	UserAgent string
	Referrer  string
	Password  string // empty unless this is a password-unlock request
}

// CacheRepository defines the Redis contract used by the redirect hot path.
// Kept separate from general cache operations so the interface stays minimal.
type CacheRepository interface {
	// GetLink retrieves a cached link by slug.
	// Returns nil, nil when cache miss (not an error).
	GetLink(ctx context.Context, slug string) (*Link, error)

	// SetLink caches a link. TTL should be set to link's remaining lifetime
	// or a default (e.g. 24h) if the link has no expiry.
	SetLink(ctx context.Context, link *Link) error

	// DeleteLink removes a link from cache (called on update/delete).
	DeleteLink(ctx context.Context, slug string) error

	// IncrRateLimit implements a sliding-window rate limiter.
	// Returns current count and whether the limit was exceeded.
	IncrRateLimit(ctx context.Context, key string, windowSecs int, limit int) (int64, bool, error)
}

// RedirectUsecase defines the business logic for the redirect hot path.
type RedirectUsecase interface {
	// Redirect resolves a slug to a destination URL.
	// This is the most performance-critical path in the system — target sub-10ms.
	// Order of operations: cache → DB → expire check → password check → geo → A/B
	Redirect(ctx context.Context, input RedirectInput) (*RedirectResult, error)

	// UnlockWithPassword validates a password for a protected link.
	UnlockWithPassword(ctx context.Context, slug string, password string) (*RedirectResult, error)

	// PublishClickEvent sends a ClickEventPayload to QStash asynchronously.
	// Must not block — called after redirect response is already sent.
	PublishClickEvent(ctx context.Context, payload ClickEventPayload) error
}
