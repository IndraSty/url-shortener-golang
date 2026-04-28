package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

// New creates a zerolog.Logger configured for the given environment.
//
// - development: human-readable console output with colors and caller info
// - production:  structured JSON output, no caller info (lower overhead)
func New(env string) zerolog.Logger {
	// Enable stack trace marshaling for errors
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	zerolog.TimeFieldFormat = time.RFC3339

	var output io.Writer

	if env == "production" {
		// JSON to stdout — consumed by Fly.io log aggregator / Grafana
		output = os.Stdout
	} else {
		// Pretty console output for local development
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}
	}

	level := zerolog.InfoLevel
	if env != "production" {
		level = zerolog.DebugLevel
	}

	return zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Str("service", "url-shortener").
		Logger()
}

// FromCtx is a helper to retrieve a logger from context.
// Returns a no-op logger if none is found, so callers never panic.
func FromCtx(ctx interface{ Value(any) any }) zerolog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(zerolog.Logger); ok {
		return l
	}
	return zerolog.Nop()
}

// WithCtx stores a logger in a context value.
// Use this to attach a request-scoped logger (e.g. with request_id) to context.
func WithCtx(ctx interface {
	Value(any) any
}, l zerolog.Logger) zerolog.Logger {
	return l
}

// loggerKey is an unexported type for context keys in this package.
// Prevents key collisions with other packages using context.WithValue.
type loggerKey struct{}
