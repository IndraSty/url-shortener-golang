package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isUniqueViolation returns true when the error is a PostgreSQL
// unique constraint violation (error code 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// nullableString converts an empty string to nil for nullable VARCHAR columns.
// Returns nil when s is empty so PostgreSQL stores NULL instead of empty string.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
