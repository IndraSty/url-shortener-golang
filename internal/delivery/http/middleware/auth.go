package middleware

import (
	"net/http"
	"strings"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/logger"
	"github.com/labstack/echo/v4"
)

const (
	// ContextKeyUserID is the Echo context key for the authenticated user ID.
	ContextKeyUserID = "user_id"

	// ContextKeyUser is the Echo context key for the full User object.
	ContextKeyUser = "user"
)

// AuthMiddleware returns an Echo middleware that validates JWT Bearer tokens.
// It extracts the user ID from the token and stores it in the Echo context.
// Protected routes must use this middleware.
func AuthMiddleware(authUsecase domain.AuthUsecase) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			log := logger.GetLogger(c)

			token, err := extractBearerToken(c.Request())
			if err != nil {
				log.Warn().Str("reason", err.Error()).Msg("auth: missing or malformed token")
				return echo.NewHTTPError(http.StatusUnauthorized, "missing or invalid authorization header")
			}

			userID, err := authUsecase.ValidateAccessToken(c.Request().Context(), token)
			if err != nil {
				log.Warn().Err(err).Msg("auth: invalid token")
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}

			// Store user ID in context for downstream handlers
			c.Set(ContextKeyUserID, userID)

			return next(c)
		}
	}
}

// APIKeyMiddleware returns an Echo middleware that validates API keys.
// API keys are passed via the X-API-Key header.
// On success, stores the full User object in context.
func APIKeyMiddleware(authUsecase domain.AuthUsecase) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			log := logger.GetLogger(c)

			rawKey := c.Request().Header.Get("X-API-Key")
			if rawKey == "" {
				log.Warn().Msg("apikey: missing X-API-Key header")
				return echo.NewHTTPError(http.StatusUnauthorized, "missing X-API-Key header")
			}

			user, err := authUsecase.ValidateAPIKey(c.Request().Context(), rawKey)
			if err != nil {
				log.Warn().Err(err).Msg("apikey: invalid key")
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid API key")
			}

			// Store both user and user ID for flexibility in handlers
			c.Set(ContextKeyUser, user)
			c.Set(ContextKeyUserID, user.ID)

			return next(c)
		}
	}
}

// AnyAuthMiddleware accepts either JWT Bearer token OR API key.
// Tries JWT first, falls back to API key.
// This allows management endpoints to be called by both browser clients (JWT)
// and programmatic clients (API key).
func AnyAuthMiddleware(authUsecase domain.AuthUsecase) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			log := logger.GetLogger(c)

			// Try JWT Bearer first
			if token, err := extractBearerToken(c.Request()); err == nil {
				userID, err := authUsecase.ValidateAccessToken(c.Request().Context(), token)
				if err == nil {
					c.Set(ContextKeyUserID, userID)
					return next(c)
				}
			}

			// Fall back to API key
			rawKey := c.Request().Header.Get("X-API-Key")
			if rawKey != "" {
				user, err := authUsecase.ValidateAPIKey(c.Request().Context(), rawKey)
				if err == nil {
					c.Set(ContextKeyUser, user)
					c.Set(ContextKeyUserID, user.ID)
					return next(c)
				}
			}

			log.Warn().Msg("auth: no valid credential found")
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
	}
}

// GetUserID retrieves the authenticated user ID from Echo context.
// Panics if called outside an authenticated route — use only in protected handlers.
func GetUserID(c echo.Context) string {
	userID, _ := c.Get(ContextKeyUserID).(string)
	return userID
}

// extractBearerToken parses the Authorization header and returns the token string.
// Returns an error if the header is missing or not in "Bearer <token>" format.
func extractBearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "missing Authorization header")
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid Authorization header format")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "empty token")
	}

	return token, nil
}
