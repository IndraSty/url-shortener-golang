package middleware

import (
	"github.com/labstack/echo/v4"
)

// SecurityHeaders returns a middleware that adds security headers to every response.
// These headers are required for OWASP compliance and protect against common web attacks.
func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()

			// Prevent MIME type sniffing — browser must respect Content-Type
			h.Set("X-Content-Type-Options", "nosniff")

			// Prevent clickjacking — disallow embedding in iframes
			h.Set("X-Frame-Options", "DENY")

			// Enable XSS filter in older browsers
			h.Set("X-XSS-Protection", "1; mode=block")

			// Force HTTPS for 1 year, include subdomains
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

			// Restrict referrer information sent to third parties
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Restrict browser features — disable unnecessary APIs
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

			// Content Security Policy — strict for API responses
			// Adjust this if you serve HTML pages (e.g. password unlock form)
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

			return next(c)
		}
	}
}

// HTTPSRedirect returns a middleware that redirects HTTP requests to HTTPS.
// Should only be active in production — Fly.io terminates TLS at the edge.
func HTTPSRedirect() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// Check both the X-Forwarded-Proto header (set by Fly.io proxy)
			// and the direct scheme
			proto := req.Header.Get("X-Forwarded-Proto")
			if proto == "http" {
				target := "https://" + req.Host + req.RequestURI
				return c.Redirect(301, target)
			}

			return next(c)
		}
	}
}
