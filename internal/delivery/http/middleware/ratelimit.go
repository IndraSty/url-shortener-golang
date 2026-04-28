package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/logger"
	"github.com/labstack/echo/v4"
)

// RedirectRateLimiter returns a strict per-IP rate limiter for the redirect endpoint.
// The redirect endpoint is the most abuse-prone path — bots can hammer it to enumerate slugs.
//
// Default: 60 requests per minute per IP.
// Uses a fixed-window counter in Redis — O(1), sub-millisecond overhead.
func RedirectRateLimiter(cacheRepo domain.CacheRepository, requestsPerMinute int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			log := logger.GetLogger(c)

			ip := c.RealIP()
			key := fmt.Sprintf("redirect:%s", ip)

			count, exceeded, err := cacheRepo.IncrRateLimit(ctx(c), key, 60, requestsPerMinute)
			if err != nil {
				// If Redis is down, fail open — don't block legitimate traffic
				log.Error().Err(err).Msg("ratelimit: redis error, failing open")
				return next(c)
			}

			// Set rate limit headers so clients can self-throttle
			c.Response().Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", requestsPerMinute))
			c.Response().Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, int64(requestsPerMinute)-count)))

			if exceeded {
				log.Warn().
					Str("ip", ip).
					Int64("count", count).
					Msg("ratelimit: redirect limit exceeded")
				return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded — try again later")
			}

			return next(c)
		}
	}
}

// APIRateLimiter returns a per-user rate limiter for management API endpoints.
// Keyed by user ID (from JWT/API key context) rather than IP —
// users behind NAT share the same IP but should have independent limits.
//
// Default: 300 requests per minute per user.
func APIRateLimiter(cacheRepo domain.CacheRepository, requestsPerMinute int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			log := logger.GetLogger(c)

			// Use user ID if authenticated, fall back to IP
			userID := GetUserID(c)
			key := fmt.Sprintf("api:%s", userID)
			if userID == "" {
				key = fmt.Sprintf("api:ip:%s", c.RealIP())
			}

			count, exceeded, err := cacheRepo.IncrRateLimit(ctx(c), key, 60, requestsPerMinute)
			if err != nil {
				log.Error().Err(err).Msg("ratelimit: redis error, failing open")
				return next(c)
			}

			c.Response().Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", requestsPerMinute))
			c.Response().Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, int64(requestsPerMinute)-count)))

			if exceeded {
				log.Warn().
					Str("user_id", userID).
					Int64("count", count).
					Msg("ratelimit: api limit exceeded")
				return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
			}

			return next(c)
		}
	}
}

// ctx extracts the standard context from Echo context.
func ctx(c echo.Context) context.Context {
	return c.Request().Context()
}

// max returns the larger of two int64 values.
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
