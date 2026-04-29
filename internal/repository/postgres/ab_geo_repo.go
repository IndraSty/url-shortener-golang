package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- A/B Test Repository ---

type abTestRepository struct {
	db *pgxpool.Pool
}

func NewABTestRepository(db *pgxpool.Pool) domain.ABTestRepository {
	return &abTestRepository{db: db}
}

func (r *abTestRepository) Create(ctx context.Context, test *domain.ABTest) error {
	query := `
		INSERT INTO ab_tests (link_id, destination_url, weight, label)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, query,
		test.LinkID,
		test.DestinationURL,
		test.Weight,
		test.Label,
	).Scan(&test.ID, &test.CreatedAt)

	if err != nil {
		return fmt.Errorf("abTestRepository.Create: %w", err)
	}
	return nil
}

func (r *abTestRepository) FindAllByLink(ctx context.Context, linkID int64) ([]*domain.ABTest, error) {
	query := `
		SELECT id, link_id, destination_url, weight, label, created_at
		FROM ab_tests
		WHERE link_id = $1
		ORDER BY created_at ASC`

	rows, err := r.db.Query(ctx, query, linkID)
	if err != nil {
		return nil, fmt.Errorf("abTestRepository.FindAllByLink: %w", err)
	}
	defer rows.Close()

	var tests []*domain.ABTest
	for rows.Next() {
		t := &domain.ABTest{}
		if err := rows.Scan(&t.ID, &t.LinkID, &t.DestinationURL,
			&t.Weight, &t.Label, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("abTestRepository.FindAllByLink scan: %w", err)
		}
		tests = append(tests, t)
	}

	return tests, rows.Err()
}

func (r *abTestRepository) FindByID(ctx context.Context, id string) (*domain.ABTest, error) {
	query := `
		SELECT id, link_id, destination_url, weight, label, created_at
		FROM ab_tests
		WHERE id = $1`

	t := &domain.ABTest{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.LinkID, &t.DestinationURL, &t.Weight, &t.Label, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrABTestNotFound
		}
		return nil, fmt.Errorf("abTestRepository.FindByID: %w", err)
	}
	return t, nil
}

func (r *abTestRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM ab_tests WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("abTestRepository.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrABTestNotFound
	}
	return nil
}

func (r *abTestRepository) SumWeightsByLink(ctx context.Context, linkID int64) (int, error) {
	query := `SELECT COALESCE(SUM(weight), 0) FROM ab_tests WHERE link_id = $1`
	var sum int
	if err := r.db.QueryRow(ctx, query, linkID).Scan(&sum); err != nil {
		return 0, fmt.Errorf("abTestRepository.SumWeightsByLink: %w", err)
	}
	return sum, nil
}

// --- Geo Rule Repository ---

type geoRuleRepository struct {
	db *pgxpool.Pool
}

func NewGeoRuleRepository(db *pgxpool.Pool) domain.GeoRuleRepository {
	return &geoRuleRepository{db: db}
}

func (r *geoRuleRepository) Create(ctx context.Context, rule *domain.GeoRule) error {
	query := `
		INSERT INTO geo_rules (link_id, country_code, destination_url, priority)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, query,
		rule.LinkID,
		rule.CountryCode,
		rule.DestinationURL,
		rule.Priority,
	).Scan(&rule.ID, &rule.CreatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrGeoRuleDuplicate
		}
		return fmt.Errorf("geoRuleRepository.Create: %w", err)
	}
	return nil
}

func (r *geoRuleRepository) FindAllByLink(ctx context.Context, linkID int64) ([]*domain.GeoRule, error) {
	query := `
		SELECT id, link_id, country_code, destination_url, priority, created_at
		FROM geo_rules
		WHERE link_id = $1
		ORDER BY priority ASC`

	rows, err := r.db.Query(ctx, query, linkID)
	if err != nil {
		return nil, fmt.Errorf("geoRuleRepository.FindAllByLink: %w", err)
	}
	defer rows.Close()

	var rules []*domain.GeoRule
	for rows.Next() {
		rule := &domain.GeoRule{}
		if err := rows.Scan(&rule.ID, &rule.LinkID, &rule.CountryCode,
			&rule.DestinationURL, &rule.Priority, &rule.CreatedAt); err != nil {
			return nil, fmt.Errorf("geoRuleRepository.FindAllByLink scan: %w", err)
		}
		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

func (r *geoRuleRepository) FindByID(ctx context.Context, id string) (*domain.GeoRule, error) {
	query := `
		SELECT id, link_id, country_code, destination_url, priority, created_at
		FROM geo_rules
		WHERE id = $1`

	rule := &domain.GeoRule{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&rule.ID, &rule.LinkID, &rule.CountryCode,
		&rule.DestinationURL, &rule.Priority, &rule.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrGeoRuleNotFound
		}
		return nil, fmt.Errorf("geoRuleRepository.FindByID: %w", err)
	}
	return rule, nil
}

func (r *geoRuleRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM geo_rules WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("geoRuleRepository.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrGeoRuleNotFound
	}
	return nil
}
