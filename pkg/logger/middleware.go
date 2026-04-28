package logger

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
)

const CtxKey = "logger"

// EchoMiddleware returns an Echo middleware that:
// 1. Attaches a request-scoped zerolog.Logger to the Echo context
// 2. Logs every request with method, path, status, latency, and request_id
func EchoMiddleware(base zerolog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			req := c.Request()
			requestID := req.Header.Get(echo.HeaderXRequestID)
			if requestID == "" {
				requestID = c.Response().Header().Get(echo.HeaderXRequestID)
			}

			// Build a request-scoped logger with common fields pre-set
			reqLogger := base.With().
				Str("request_id", requestID).
				Str("method", req.Method).
				Str("path", req.URL.Path).
				Str("ip", c.RealIP()).
				Logger()

			// Attach to Echo context so handlers can retrieve it
			c.Set(CtxKey, reqLogger)

			// Process request
			err := next(c)

			// Log after response is written
			latency := time.Since(start)
			status := c.Response().Status

			event := reqLogger.Info()
			if status >= 500 {
				event = reqLogger.Error()
			} else if status >= 400 {
				event = reqLogger.Warn()
			}

			event.
				Int("status", status).
				Dur("latency_ms", latency).
				Int64("bytes_out", c.Response().Size).
				Msg("request completed")

			return err
		}
	}
}

// GetLogger retrieves the request-scoped logger from Echo context.
// Falls back to a no-op logger if not found (safe to call anywhere).
func GetLogger(c echo.Context) zerolog.Logger {
	if l, ok := c.Get(CtxKey).(zerolog.Logger); ok {
		return l
	}
	return zerolog.Nop()
}
