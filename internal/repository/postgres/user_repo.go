package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userRepository struct {
	db *pgxpool.Pool
}

// NewUserRepository creates a new PostgreSQL-backed user repository.
func NewUserRepository(db *pgxpool.Pool) domain.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (id, email, password_hash, api_key, plan, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		user.Email,
		user.PasswordHash,
		user.APIKey,
		user.Plan,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		// Check for unique constraint violation on email
		if isUniqueViolation(err) {
			return domain.ErrEmailAlreadyExists
		}
		return fmt.Errorf("userRepository.Create: %w", err)
	}

	return nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, api_key, plan, created_at, updated_at
		FROM users
		WHERE email = $1`

	user := &domain.User{}
	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.APIKey,
		&user.Plan,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("userRepository.FindByEmail: %w", err)
	}

	return user, nil
}

func (r *userRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, api_key, plan, created_at, updated_at
		FROM users
		WHERE id = $1`

	user := &domain.User{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.APIKey,
		&user.Plan,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("userRepository.FindByID: %w", err)
	}

	return user, nil
}

func (r *userRepository) FindByAPIKey(ctx context.Context, hashedKey string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, api_key, plan, created_at, updated_at
		FROM users
		WHERE api_key = $1`

	user := &domain.User{}
	err := r.db.QueryRow(ctx, query, hashedKey).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.APIKey,
		&user.Plan,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("userRepository.FindByAPIKey: %w", err)
	}

	return user, nil
}
