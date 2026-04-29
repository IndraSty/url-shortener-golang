package domain

import (
	"context"
	"time"
)

// Link is the core entity — represents one shortened URL.
// id (bigserial) is base62-encoded to produce the default slug.
// Custom slugs are stored directly in the slug field.
type Link struct {
	ID             int64
	UserID         string
	Slug           string
	DestinationURL string
	Title          string
	PasswordHash   string // empty string means no password protection
	IsActive       bool
	ClickCount     int64      // denormalized counter — fast reads for dashboard
	ExpiredAt      *time.Time // nil means never expires
	CreatedAt      time.Time
	UpdatedAt      time.Time

	// Populated on demand — not stored in links table
	ABTests  []*ABTest
	GeoRules []*GeoRule
}

// IsExpired returns true if the link has a set expiry that has passed.
func (l *Link) IsExpired() bool {
	if l.ExpiredAt == nil {
		return false
	}
	return time.Now().After(*l.ExpiredAt)
}

// IsPasswordProtected returns true when the link requires a password.
func (l *Link) IsPasswordProtected() bool {
	return l.PasswordHash != ""
}

// ABTest represents one destination variant in an A/B test.
// All variants for a link must have weights summing to exactly 100.
type ABTest struct {
	ID             string // UUID
	LinkID         int64
	DestinationURL string
	Weight         int    // 1–100, percentage share of traffic
	Label          string // human-readable name e.g. "variant-a", "control"
	CreatedAt      time.Time
}

// GeoRule redirects visitors from a specific country to a different URL.
// country_code is ISO 3166-1 alpha-2 (2 uppercase letters).
type GeoRule struct {
	ID             string // UUID
	LinkID         int64
	CountryCode    string // e.g. "ID", "US", "SG"
	DestinationURL string
	Priority       int // lower value = evaluated first
	CreatedAt      time.Time
}

// --- Input types (usecase layer input, not domain entities) ---

// CreateLinkInput holds validated fields for creating a new link.
type CreateLinkInput struct {
	UserID         string
	DestinationURL string
	Title          string
	CustomSlug     string     // optional — auto-generated from ID if empty
	Password       string     // optional — bcrypt-hashed before storage
	ExpiredAt      *time.Time // optional
}

// UpdateLinkInput holds fields that can be patched on an existing link.
// Pointer fields allow distinguishing "not provided" from "set to zero value".
type UpdateLinkInput struct {
	DestinationURL *string
	Title          *string
	Password       *string // set to "" to remove password protection
	IsActive       *bool
	ExpiredAt      *time.Time // set to nil to remove expiry
}

// CreateABTestInput holds validated fields for adding an A/B variant.
type CreateABTestInput struct {
	LinkID         int64
	DestinationURL string
	Weight         int
	Label          string
}

// CreateGeoRuleInput holds validated fields for adding a geo rule.
type CreateGeoRuleInput struct {
	LinkID         int64
	CountryCode    string
	DestinationURL string
	Priority       int
}

// --- Repository interfaces ---

// LinkRepository defines the persistence contract for links.
type LinkRepository interface {
	// Create inserts a new link and returns it with ID + slug populated.
	Create(ctx context.Context, link *Link) error

	// FindBySlug returns a link by slug with ABTests and GeoRules populated.
	// Returns ErrLinkNotFound if no matching active link exists.
	FindBySlug(ctx context.Context, slug string) (*Link, error)

	// FindByID returns a link by its numeric ID.
	// Returns ErrLinkNotFound if not found.
	FindByID(ctx context.Context, id int64) (*Link, error)

	// FindAllByUser returns all links for a user, newest first.
	FindAllByUser(ctx context.Context, userID string, limit, offset int) ([]*Link, int64, error)

	// Update applies the given fields to an existing link.
	Update(ctx context.Context, id int64, input UpdateLinkInput) (*Link, error)

	// UpdateSlug sets the slug field directly — used after ID is known from insert.
	UpdateSlug(ctx context.Context, id int64, slug string) error

	// Delete soft-deletes a link by setting is_active = false.
	Delete(ctx context.Context, id int64) error

	// IncrementClickCount atomically increments the denormalized click counter.
	IncrementClickCount(ctx context.Context, id int64) error

	// SlugExists checks whether a slug is already taken.
	SlugExists(ctx context.Context, slug string) (bool, error)
}

// ABTestRepository defines persistence for A/B test variants.
type ABTestRepository interface {
	// Create inserts a new A/B variant.
	Create(ctx context.Context, test *ABTest) error

	// FindAllByLink returns all variants for a link.
	FindAllByLink(ctx context.Context, linkID int64) ([]*ABTest, error)

	// FindByID returns a specific variant.
	FindByID(ctx context.Context, id string) (*ABTest, error)

	// Delete removes a variant by ID.
	Delete(ctx context.Context, id string) error

	// SumWeightsByLink returns the total weight of all variants for a link.
	// Used to validate that weights sum to 100 before adding a new variant.
	SumWeightsByLink(ctx context.Context, linkID int64) (int, error)
}

// GeoRuleRepository defines persistence for geo rules.
type GeoRuleRepository interface {
	// Create inserts a new geo rule.
	Create(ctx context.Context, rule *GeoRule) error

	// FindAllByLink returns all geo rules for a link, ordered by priority.
	FindAllByLink(ctx context.Context, linkID int64) ([]*GeoRule, error)

	// FindByID returns a specific geo rule.
	FindByID(ctx context.Context, id string) (*GeoRule, error)

	// Delete removes a geo rule by ID.
	Delete(ctx context.Context, id string) error
}

// --- Usecase interfaces ---

// LinkUsecase defines business logic for link management.
type LinkUsecase interface {
	Create(ctx context.Context, input CreateLinkInput) (*Link, error)
	GetByID(ctx context.Context, id int64, userID string) (*Link, error)
	GetAllByUser(ctx context.Context, userID string, limit, offset int) ([]*Link, int64, error)
	Update(ctx context.Context, id int64, userID string, input UpdateLinkInput) (*Link, error)
	Delete(ctx context.Context, id int64, userID string) error
	GenerateQRCode(ctx context.Context, id int64, userID string) ([]byte, error)

	// A/B test management
	CreateABTest(ctx context.Context, input CreateABTestInput, userID string) (*ABTest, error)
	GetABTests(ctx context.Context, linkID int64, userID string) ([]*ABTest, error)
	DeleteABTest(ctx context.Context, linkID int64, variantID string, userID string) error

	// Geo rule management
	CreateGeoRule(ctx context.Context, input CreateGeoRuleInput, userID string) (*GeoRule, error)
	GetGeoRules(ctx context.Context, linkID int64, userID string) ([]*GeoRule, error)
	DeleteGeoRule(ctx context.Context, linkID int64, ruleID string, userID string) error
}
