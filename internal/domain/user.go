package domain

import (
	"context"
	"time"
)

// User represents an authenticated account in the system.
// api_key is stored as SHA-256 hex hash in the database —
// the plaintext is returned once at registration and never stored.
type User struct {
	ID           string // UUID
	Email        string
	PasswordHash string // bcrypt hash
	APIKey       string // SHA-256 hex hash (64 chars) — never plaintext
	Plan         string // "free" | "pro"
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RegisterInput contains validated fields for user registration.
type RegisterInput struct {
	Email    string
	Password string // plaintext — hashed in usecase before storage
}

// LoginInput contains fields for authentication.
type LoginInput struct {
	Email    string
	Password string // plaintext — compared against bcrypt hash
}

// AuthTokens holds the JWT pair returned after successful auth.
type AuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64 // access token expiry in seconds
}

// UserRepository defines the persistence contract for users.
type UserRepository interface {
	// Create inserts a new user and returns it with ID populated.
	Create(ctx context.Context, user *User) error

	// FindByEmail returns a user by email. Returns ErrUserNotFound if missing.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// FindByID returns a user by UUID. Returns ErrUserNotFound if missing.
	FindByID(ctx context.Context, id string) (*User, error)

	// FindByAPIKey returns a user by hashed API key. Returns ErrUserNotFound if missing.
	FindByAPIKey(ctx context.Context, hashedKey string) (*User, error)
}

// AuthUsecase defines the business logic contract for authentication.
type AuthUsecase interface {
	// Register creates a new user account and returns auth tokens.
	Register(ctx context.Context, input RegisterInput) (*AuthTokens, error)

	// Login authenticates a user and returns auth tokens.
	Login(ctx context.Context, input LoginInput) (*AuthTokens, error)

	// ValidateAccessToken parses and validates a JWT access token.
	// Returns the user ID encoded in the token claims.
	ValidateAccessToken(ctx context.Context, token string) (userID string, err error)

	// ValidateAPIKey looks up a user by SHA-256 hash of the provided key.
	ValidateAPIKey(ctx context.Context, rawKey string) (*User, error)
}
