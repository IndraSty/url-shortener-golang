package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/IndraSty/url-shortener-golang/config"
	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor for bcrypt hashing.
// 12 is the recommended minimum for production — higher = slower brute force.
// Don't increase beyond 14 on free-tier servers (CPU-bound).
const bcryptCost = 12

type authUsecase struct {
	userRepo domain.UserRepository
	jwtCfg   config.JWTConfig
}

// NewAuthUsecase creates the authentication usecase.
func NewAuthUsecase(userRepo domain.UserRepository, jwtCfg config.JWTConfig) domain.AuthUsecase {
	return &authUsecase{
		userRepo: userRepo,
		jwtCfg:   jwtCfg,
	}
}

func (u *authUsecase) Register(ctx context.Context, input domain.RegisterInput) (*domain.AuthTokens, error) {
	// Hash password — bcrypt is intentionally slow to resist brute force
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("authUsecase.Register hash password: %w", err)
	}

	// Generate a cryptographically random API key (32 bytes = 64 hex chars)
	rawAPIKey, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("authUsecase.Register generate api key: %w", err)
	}

	// Store only the SHA-256 hash of the API key — plaintext is returned once and discarded
	apiKeyHash := sha256Hash(rawAPIKey)

	user := &domain.User{
		Email:        input.Email,
		PasswordHash: string(passwordHash),
		APIKey:       apiKeyHash,
		Plan:         "free",
	}

	if err := u.userRepo.Create(ctx, user); err != nil {
		return nil, err // ErrEmailAlreadyExists propagates as-is
	}

	tokens, err := u.generateTokenPair(user.ID)
	if err != nil {
		return nil, fmt.Errorf("authUsecase.Register generate tokens: %w", err)
	}

	// NOTE: rawAPIKey is returned ONCE here and never stored in plaintext.
	// The caller (HTTP handler) must include it in the response body.
	// After this point it's gone — user must regenerate if lost.
	tokens.AccessToken = fmt.Sprintf("%s|apikey:%s", tokens.AccessToken, rawAPIKey)

	return tokens, nil
}

func (u *authUsecase) Login(ctx context.Context, input domain.LoginInput) (*domain.AuthTokens, error) {
	user, err := u.userRepo.FindByEmail(ctx, input.Email)
	if err != nil {
		// Map ErrUserNotFound to ErrInvalidCredentials — don't reveal which field was wrong
		if err == domain.ErrUserNotFound {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("authUsecase.Login find user: %w", err)
	}

	// Constant-time comparison — bcrypt.CompareHashAndPassword handles this internally
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	return u.generateTokenPair(user.ID)
}

func (u *authUsecase) ValidateAccessToken(_ context.Context, tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(u.jwtCfg.AccessSecret), nil
	})
	if err != nil {
		return "", domain.ErrUnauthorized
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", domain.ErrUnauthorized
	}

	userID, ok := claims["sub"].(string)
	if !ok || userID == "" {
		return "", domain.ErrUnauthorized
	}

	return userID, nil
}

func (u *authUsecase) ValidateAPIKey(ctx context.Context, rawKey string) (*domain.User, error) {
	// Hash the incoming key and look up — we never compare against plaintext
	hashedKey := sha256Hash(rawKey)

	user, err := u.userRepo.FindByAPIKey(ctx, hashedKey)
	if err != nil {
		if err == domain.ErrUserNotFound {
			return nil, domain.ErrUnauthorized
		}
		return nil, fmt.Errorf("authUsecase.ValidateAPIKey: %w", err)
	}

	return user, nil
}

// --- JWT helpers ---

type jwtClaims struct {
	jwt.RegisteredClaims
}

func (u *authUsecase) generateTokenPair(userID string) (*domain.AuthTokens, error) {
	now := time.Now()

	// Access token — short lived (15 min default)
	accessClaims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(u.jwtCfg.AccessExpiry)),
			Issuer:    "url-shortener",
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenStr, err := accessToken.SignedString([]byte(u.jwtCfg.AccessSecret))
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	// Refresh token — long lived (7 days default)
	refreshClaims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(u.jwtCfg.RefreshExpiry)),
			Issuer:    "url-shortener",
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenStr, err := refreshToken.SignedString([]byte(u.jwtCfg.RefreshSecret))
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &domain.AuthTokens{
		AccessToken:  accessTokenStr,
		RefreshToken: refreshTokenStr,
		ExpiresIn:    int64(u.jwtCfg.AccessExpiry.Seconds()),
	}, nil
}

// --- Crypto helpers ---

// generateRandomHex returns a cryptographically secure random hex string of n bytes.
func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generateRandomHex: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// sha256Hash returns the SHA-256 hex hash of a string.
func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
