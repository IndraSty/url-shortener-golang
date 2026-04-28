package postgres

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres driver
	_ "github.com/golang-migrate/migrate/v4/source/file"       // file source
)

// RunMigrations applies all pending up migrations from the migrations directory.
// It is safe to call on every startup — already-applied migrations are skipped.
func RunMigrations(databaseURL, migrationsPath string) error {
	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		databaseURL,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		// ErrNoChange means all migrations already applied — not an error
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}
