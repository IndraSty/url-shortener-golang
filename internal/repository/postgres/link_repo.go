package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type linkRepository struct {
	db *pgxpool.Pool
}

// NewLinkRepository creates a new PostgreSQL-backed link repository.
func NewLinkRepository(db *pgxpool.Pool) domain.LinkRepository {
	return &linkRepository{db: db}
}

func (r *linkRepository) Create(ctx context.Context, link *domain.Link) error {
	query := `
		INSERT INTO links
			(user_id, slug, destination_url, title, password_hash, is_active, expired_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, click_count, created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		link.UserID,
		link.Slug,
		link.DestinationURL,
		link.Title,
		nullableString(link.PasswordHash),
		link.IsActive,
		link.ExpiredAt,
	).Scan(
		&link.ID,
		&link.ClickCount,
		&link.CreatedAt,
		&link.UpdatedAt,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugAlreadyExists
		}
		return fmt.Errorf("linkRepository.Create: %w", err)
	}

	return nil
}

func (r *linkRepository) FindBySlug(ctx context.Context, slug string) (*domain.Link, error) {
	// Single query — join is not used here intentionally.
	// ABTests and GeoRules are loaded separately to keep this query fast
	// and avoid result set explosion from multiple joins on the hot redirect path.
	query := `
		SELECT id, user_id, slug, destination_url, title,
		       COALESCE(password_hash, ''), is_active, click_count,
		       expired_at, created_at, updated_at
		FROM links
		WHERE slug = $1`

	link := &domain.Link{}
	err := r.db.QueryRow(ctx, query, slug).Scan(
		&link.ID,
		&link.UserID,
		&link.Slug,
		&link.DestinationURL,
		&link.Title,
		&link.PasswordHash,
		&link.IsActive,
		&link.ClickCount,
		&link.ExpiredAt,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrLinkNotFound
		}
		return nil, fmt.Errorf("linkRepository.FindBySlug: %w", err)
	}

	// Load AB tests and geo rules in parallel goroutines
	abTests, err := r.findABTestsByLinkID(ctx, link.ID)
	if err != nil {
		return nil, err
	}
	link.ABTests = abTests

	geoRules, err := r.findGeoRulesByLinkID(ctx, link.ID)
	if err != nil {
		return nil, err
	}
	link.GeoRules = geoRules

	return link, nil
}

func (r *linkRepository) FindByID(ctx context.Context, id int64) (*domain.Link, error) {
	query := `
		SELECT id, user_id, slug, destination_url, title,
		       COALESCE(password_hash, ''), is_active, click_count,
		       expired_at, created_at, updated_at
		FROM links
		WHERE id = $1`

	link := &domain.Link{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&link.ID,
		&link.UserID,
		&link.Slug,
		&link.DestinationURL,
		&link.Title,
		&link.PasswordHash,
		&link.IsActive,
		&link.ClickCount,
		&link.ExpiredAt,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrLinkNotFound
		}
		return nil, fmt.Errorf("linkRepository.FindByID: %w", err)
	}

	abTests, err := r.findABTestsByLinkID(ctx, link.ID)
	if err != nil {
		return nil, err
	}
	link.ABTests = abTests

	geoRules, err := r.findGeoRulesByLinkID(ctx, link.ID)
	if err != nil {
		return nil, err
	}
	link.GeoRules = geoRules

	return link, nil
}

func (r *linkRepository) FindAllByUser(ctx context.Context, userID string, limit, offset int) ([]*domain.Link, int64, error) {
	// Count query for pagination
	countQuery := `SELECT COUNT(*) FROM links WHERE user_id = $1`
	var total int64
	if err := r.db.QueryRow(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("linkRepository.FindAllByUser count: %w", err)
	}

	query := `
		SELECT id, user_id, slug, destination_url, title,
		       COALESCE(password_hash, ''), is_active, click_count,
		       expired_at, created_at, updated_at
		FROM links
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("linkRepository.FindAllByUser: %w", err)
	}
	defer rows.Close()

	var links []*domain.Link
	for rows.Next() {
		link := &domain.Link{}
		if err := rows.Scan(
			&link.ID, &link.UserID, &link.Slug, &link.DestinationURL,
			&link.Title, &link.PasswordHash, &link.IsActive, &link.ClickCount,
			&link.ExpiredAt, &link.CreatedAt, &link.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("linkRepository.FindAllByUser scan: %w", err)
		}
		links = append(links, link)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("linkRepository.FindAllByUser rows: %w", err)
	}

	return links, total, nil
}

func (r *linkRepository) Update(ctx context.Context, id int64, input domain.UpdateLinkInput) (*domain.Link, error) {
	// Build dynamic UPDATE — only set fields that were provided
	// We use a simple approach: fetch then update in a transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("linkRepository.Update begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	query := `
		UPDATE links SET
			destination_url = COALESCE($1, destination_url),
			title           = COALESCE($2, title),
			password_hash   = CASE WHEN $3::boolean THEN $4 ELSE password_hash END,
			is_active       = COALESCE($5, is_active),
			expired_at      = CASE WHEN $6::boolean THEN $7 ELSE expired_at END,
			updated_at      = NOW()
		WHERE id = $8
		RETURNING id, user_id, slug, destination_url, title,
		          COALESCE(password_hash, ''), is_active, click_count,
		          expired_at, created_at, updated_at`

	// Use sentinel booleans to distinguish "not provided" from "set to null"
	passwordProvided := input.Password != nil
	var passwordHash *string
	if passwordProvided {
		passwordHash = input.Password
	}

	expiredAtProvided := input.ExpiredAt != nil

	link := &domain.Link{}
	err = tx.QueryRow(ctx, query,
		input.DestinationURL,
		input.Title,
		passwordProvided,
		passwordHash,
		input.IsActive,
		expiredAtProvided,
		input.ExpiredAt,
		id,
	).Scan(
		&link.ID, &link.UserID, &link.Slug, &link.DestinationURL,
		&link.Title, &link.PasswordHash, &link.IsActive, &link.ClickCount,
		&link.ExpiredAt, &link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrLinkNotFound
		}
		return nil, fmt.Errorf("linkRepository.Update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("linkRepository.Update commit: %w", err)
	}

	return link, nil
}

func (r *linkRepository) UpdateSlug(ctx context.Context, id int64, slug string) error {
	query := `UPDATE links SET slug = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.db.Exec(ctx, query, slug, id)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugAlreadyExists
		}
		return fmt.Errorf("linkRepository.UpdateSlug: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrLinkNotFound
	}
	return nil
}

func (r *linkRepository) Delete(ctx context.Context, id int64) error {
	// Soft delete — set is_active = false
	// Hard delete would break analytics history
	query := `UPDATE links SET is_active = FALSE, updated_at = NOW() WHERE id = $1`

	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("linkRepository.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrLinkNotFound
	}

	return nil
}

func (r *linkRepository) IncrementClickCount(ctx context.Context, id int64) error {
	query := `UPDATE links SET click_count = click_count + 1 WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("linkRepository.IncrementClickCount: %w", err)
	}
	return nil
}

func (r *linkRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM links WHERE slug = $1)`
	var exists bool
	if err := r.db.QueryRow(ctx, query, slug).Scan(&exists); err != nil {
		return false, fmt.Errorf("linkRepository.SlugExists: %w", err)
	}
	return exists, nil
}

// --- Internal helpers ---

func (r *linkRepository) findABTestsByLinkID(ctx context.Context, linkID int64) ([]*domain.ABTest, error) {
	query := `
		SELECT id, link_id, destination_url, weight, label, created_at
		FROM ab_tests
		WHERE link_id = $1
		ORDER BY created_at ASC`

	rows, err := r.db.Query(ctx, query, linkID)
	if err != nil {
		return nil, fmt.Errorf("findABTestsByLinkID: %w", err)
	}
	defer rows.Close()

	var tests []*domain.ABTest
	for rows.Next() {
		t := &domain.ABTest{}
		if err := rows.Scan(&t.ID, &t.LinkID, &t.DestinationURL, &t.Weight, &t.Label, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("findABTestsByLinkID scan: %w", err)
		}
		tests = append(tests, t)
	}

	return tests, rows.Err()
}

func (r *linkRepository) findGeoRulesByLinkID(ctx context.Context, linkID int64) ([]*domain.GeoRule, error) {
	query := `
		SELECT id, link_id, country_code, destination_url, priority, created_at
		FROM geo_rules
		WHERE link_id = $1
		ORDER BY priority ASC`

	rows, err := r.db.Query(ctx, query, linkID)
	if err != nil {
		return nil, fmt.Errorf("findGeoRulesByLinkID: %w", err)
	}
	defer rows.Close()

	var rules []*domain.GeoRule
	for rows.Next() {
		rule := &domain.GeoRule{}
		if err := rows.Scan(&rule.ID, &rule.LinkID, &rule.CountryCode,
			&rule.DestinationURL, &rule.Priority, &rule.CreatedAt); err != nil {
			return nil, fmt.Errorf("findGeoRulesByLinkID scan: %w", err)
		}
		rules = append(rules, rule)
	}

	return rules, rows.Err()
}
