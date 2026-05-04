package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/IndraSty/url-shortener-golang/pkg/metrics"
	"github.com/labstack/echo/v4"
)

// PrometheusMiddleware records HTTP request metrics for every request.
// Must be registered early in the middleware chain to capture all requests.
//
// Recorded metrics:
//   - http_requests_total{method, path, status}
//   - http_request_duration_seconds{method, path}
//   - http_response_size_bytes{method, path}
func PrometheusMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)

			// Capture after handler runs
			duration := time.Since(start).Seconds()
			status := c.Response().Status
			method := c.Request().Method

			// Normalize path — use route template not actual path.
			// This prevents high-cardinality metrics from unique slugs.
			// e.g. "/:slug" not "/abc123", "/xyz789"
			path := normalizePath(c)

			metrics.HTTPRequestsTotal.WithLabelValues(
				method,
				path,
				strconv.Itoa(status),
			).Inc()

			metrics.HTTPRequestDuration.WithLabelValues(
				method,
				path,
			).Observe(duration)

			metrics.HTTPResponseSize.WithLabelValues(
				method,
				path,
			).Observe(float64(c.Response().Size))

			return err
		}
	}
}

// RedirectMetricsMiddleware records redirect-specific metrics.
// Should only wrap the /:slug route, not all routes.
func RedirectMetricsMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)

			latency := time.Since(start).Seconds()
			metrics.RedirectLatency.Observe(latency)

			// Label by outcome
			status := c.Response().Status
			result := classifyRedirectResult(status, err)
			metrics.RedirectsTotal.WithLabelValues(result).Inc()

			return err
		}
	}
}

// classifyRedirectResult maps HTTP status + error to a meaningful label.
func classifyRedirectResult(status int, err error) string {
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			switch he.Code {
			case http.StatusNotFound:
				return "not_found"
			case http.StatusUnauthorized:
				return "password_required"
			case http.StatusTooManyRequests:
				return "rate_limited"
			}
		}
		return "error"
	}

	switch status {
	case http.StatusMovedPermanently:
		return "redirect_301"
	case http.StatusFound:
		return "redirect_302"
	default:
		return "other"
	}
}

// normalizePath returns the Echo route template for a request.
// Falls back to a bucketed path if no route matches.
func normalizePath(c echo.Context) string {
	// Echo stores the matched route pattern in the context
	path := c.Path()
	if path == "" {
		path = "unknown"
	}
	return path
}
