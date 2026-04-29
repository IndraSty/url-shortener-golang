package domain

import "errors"

// Sentinel errors — use errors.Is() to check these across layer boundaries.
// Each error maps to a specific HTTP status in the delivery layer.
// We define them here so no layer needs to import another layer's error types.

var (
	// --- Link errors ---

	// ErrLinkNotFound is returned when a slug doesn't exist OR is expired.
	// Intentionally the same error for both cases — prevents slug enumeration attacks.
	ErrLinkNotFound = errors.New("link not found")

	// ErrLinkExpired is returned internally but mapped to ErrLinkNotFound in HTTP layer.
	ErrLinkExpired = errors.New("link has expired")

	// ErrLinkInactive is returned when link exists but is_active = false.
	ErrLinkInactive = errors.New("link is inactive")

	// ErrSlugAlreadyExists is returned when a custom slug is already taken.
	ErrSlugAlreadyExists = errors.New("slug already exists")

	// ErrPasswordRequired is returned when a link is protected and no password given.
	ErrPasswordRequired = errors.New("password required")

	// ErrInvalidPassword is returned when the submitted password is wrong.
	ErrInvalidPassword = errors.New("invalid password")

	// --- Auth errors ---

	// ErrUserNotFound is returned when email doesn't match any user.
	ErrUserNotFound = errors.New("user not found")

	// ErrEmailAlreadyExists is returned on duplicate registration.
	ErrEmailAlreadyExists = errors.New("email already exists")

	// ErrInvalidCredentials is returned for wrong email/password combo.
	// Intentionally vague — don't reveal which field was wrong.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrUnauthorized is returned when JWT or API key is missing/invalid.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when user tries to access another user's resource.
	ErrForbidden = errors.New("forbidden")

	// --- A/B test errors ---

	// ErrABTestNotFound is returned when variant ID doesn't exist.
	ErrABTestNotFound = errors.New("ab test not found")

	// ErrInvalidABWeight is returned when weights don't sum to 100.
	ErrInvalidABWeight = errors.New("a/b test weights must sum to 100")

	// ErrABTestConflict is returned when trying to add A/B tests to a link
	// that already has geo rules pointing to a different destination.
	ErrABTestConflict = errors.New("cannot mix a/b tests with geo-only rules")

	// --- Geo rule errors ---

	// ErrGeoRuleNotFound is returned when a geo rule ID doesn't exist.
	ErrGeoRuleNotFound = errors.New("geo rule not found")

	// ErrGeoRuleDuplicate is returned when country already has a rule for this link.
	ErrGeoRuleDuplicate = errors.New("geo rule already exists for this country")

	// --- Validation errors ---

	// ErrInvalidSlug is returned when a custom slug contains invalid characters.
	ErrInvalidSlug = errors.New("slug must be alphanumeric (a-z, A-Z, 0-9, hyphen)")

	// ErrInvalidURL is returned when destination URL format is invalid.
	ErrInvalidURL = errors.New("invalid destination URL")

	// ErrInvalidInput is a generic validation error with a message.
	ErrInvalidInput = errors.New("invalid input")

	// --- Rate limit ---

	// ErrRateLimitExceeded is returned when a client hits the rate limit.
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// DomainError wraps a sentinel error with additional context.
// Use this when you need to attach a human-readable message to a known error type.
//
// Example:
//
//	return &DomainError{Code: domain.ErrInvalidInput, Message: "weight must be between 1 and 100"}
type DomainError struct {
	Code    error  // sentinel error for errors.Is() checks
	Message string // human-readable message for API response
}

func (e *DomainError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code.Error()
}

// Unwrap allows errors.Is() to unwrap through DomainError to the sentinel Code.
func (e *DomainError) Unwrap() error {
	return e.Code
}

// NewError creates a DomainError with a custom message.
func NewError(code error, message string) *DomainError {
	return &DomainError{Code: code, Message: message}
}
