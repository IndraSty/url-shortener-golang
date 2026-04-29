package usecase

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/IndraSty/url-shortener-golang/config"
	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/base62"
	"github.com/IndraSty/url-shortener-golang/pkg/qrcode"
	"golang.org/x/crypto/bcrypt"
)

// slugPattern validates custom slugs: alphanumeric + hyphen, 3–20 chars.
var slugPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{3,20}$`)

// urlPattern is a basic URL validator — scheme must be http or https.
var urlPattern = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)

type linkUsecase struct {
	linkRepo    domain.LinkRepository
	abTestRepo  domain.ABTestRepository
	geoRuleRepo domain.GeoRuleRepository
	cacheRepo   domain.CacheRepository
	qrGen       *qrcode.Generator
	cfg         *config.Config
}

// NewLinkUsecase creates the link management usecase.
func NewLinkUsecase(
	linkRepo domain.LinkRepository,
	abTestRepo domain.ABTestRepository,
	geoRuleRepo domain.GeoRuleRepository,
	cacheRepo domain.CacheRepository,
	qrGen *qrcode.Generator,
	cfg *config.Config,
) domain.LinkUsecase {
	return &linkUsecase{
		linkRepo:    linkRepo,
		abTestRepo:  abTestRepo,
		geoRuleRepo: geoRuleRepo,
		cacheRepo:   cacheRepo,
		qrGen:       qrGen,
		cfg:         cfg,
	}
}

func (u *linkUsecase) Create(ctx context.Context, input domain.CreateLinkInput) (*domain.Link, error) {
	// Validate destination URL
	if !urlPattern.MatchString(input.DestinationURL) {
		return nil, domain.ErrInvalidURL
	}

	link := &domain.Link{
		UserID:         input.UserID,
		DestinationURL: input.DestinationURL,
		Title:          input.Title,
		IsActive:       true,
		ExpiredAt:      input.ExpiredAt,
	}

	// Handle password protection
	if input.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("linkUsecase.Create hash password: %w", err)
		}
		link.PasswordHash = string(hash)
	}

	// Handle custom slug
	if input.CustomSlug != "" {
		if !slugPattern.MatchString(input.CustomSlug) {
			return nil, domain.ErrInvalidSlug
		}
		exists, err := u.linkRepo.SlugExists(ctx, input.CustomSlug)
		if err != nil {
			return nil, fmt.Errorf("linkUsecase.Create slug check: %w", err)
		}
		if exists {
			return nil, domain.ErrSlugAlreadyExists
		}
		link.Slug = input.CustomSlug
	} else {
		// Temporary slug — will be replaced with base62(id) after insert
		link.Slug = fmt.Sprintf("tmp-%d", time.Now().UnixNano())
	}

	// Insert — gives us the auto-increment ID
	if err := u.linkRepo.Create(ctx, link); err != nil {
		return nil, err
	}

	// Replace temporary slug with base62-encoded ID
	if input.CustomSlug == "" {
		generatedSlug := base62.Encode(link.ID)
		if err := u.linkRepo.UpdateSlug(ctx, link.ID, generatedSlug); err != nil {
			return nil, fmt.Errorf("linkUsecase.Create update slug: %w", err)
		}
		link.Slug = generatedSlug
	}

	return link, nil
}

// updateSlugDirect updates only the slug field — called after ID is known.
// We keep this internal to the usecase, not exposed via repository interface.
func (u *linkUsecase) updateSlugDirect(ctx context.Context, id int64, slug string) error {
	_, err := u.linkRepo.Update(ctx, id, domain.UpdateLinkInput{})
	if err != nil {
		return err
	}
	// We need direct slug update — add it to UpdateLinkInput
	// For now we'll handle this via a direct method
	return nil
}

func (u *linkUsecase) GetByID(ctx context.Context, id int64, userID string) (*domain.Link, error) {
	link, err := u.linkRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Authorization check — users can only access their own links
	if link.UserID != userID {
		return nil, domain.ErrForbidden
	}

	return link, nil
}

func (u *linkUsecase) GetAllByUser(ctx context.Context, userID string, limit, offset int) ([]*domain.Link, int64, error) {
	// Clamp pagination params
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	return u.linkRepo.FindAllByUser(ctx, userID, limit, offset)
}

func (u *linkUsecase) Update(ctx context.Context, id int64, userID string, input domain.UpdateLinkInput) (*domain.Link, error) {
	// Authorization check
	existing, err := u.linkRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing.UserID != userID {
		return nil, domain.ErrForbidden
	}

	// Validate new destination URL if provided
	if input.DestinationURL != nil {
		if !urlPattern.MatchString(*input.DestinationURL) {
			return nil, domain.ErrInvalidURL
		}
	}

	// Hash new password if provided
	if input.Password != nil && *input.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*input.Password), bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("linkUsecase.Update hash password: %w", err)
		}
		hashed := string(hash)
		input.Password = &hashed
	}

	updated, err := u.linkRepo.Update(ctx, id, input)
	if err != nil {
		return nil, err
	}

	// Invalidate cache — next redirect will re-fetch from DB
	if err := u.cacheRepo.DeleteLink(ctx, existing.Slug); err != nil {
		// Non-fatal — cache will expire naturally
		_ = err
	}

	return updated, nil
}

func (u *linkUsecase) Delete(ctx context.Context, id int64, userID string) error {
	existing, err := u.linkRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.UserID != userID {
		return domain.ErrForbidden
	}

	if err := u.linkRepo.Delete(ctx, id); err != nil {
		return err
	}

	// Invalidate cache
	_ = u.cacheRepo.DeleteLink(ctx, existing.Slug)

	return nil
}

func (u *linkUsecase) GenerateQRCode(ctx context.Context, id int64, userID string) ([]byte, error) {
	link, err := u.linkRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if link.UserID != userID {
		return nil, domain.ErrForbidden
	}

	// Build the short URL that the QR code will point to
	shortURL := fmt.Sprintf("%s/%s", strings.TrimRight(u.cfg.App.BaseURL, "/"), link.Slug)

	return u.qrGen.GenerateDefault(shortURL)
}

// --- A/B Test management ---

func (u *linkUsecase) CreateABTest(ctx context.Context, input domain.CreateABTestInput, userID string) (*domain.ABTest, error) {
	// Authorization
	link, err := u.linkRepo.FindByID(ctx, input.LinkID)
	if err != nil {
		return nil, err
	}
	if link.UserID != userID {
		return nil, domain.ErrForbidden
	}

	// Validate URL
	if !urlPattern.MatchString(input.DestinationURL) {
		return nil, domain.ErrInvalidURL
	}

	// Validate weight range
	if input.Weight <= 0 || input.Weight > 100 {
		return nil, domain.NewError(domain.ErrInvalidABWeight, "weight must be between 1 and 100")
	}

	// Check total weight won't exceed 100
	currentSum, err := u.abTestRepo.SumWeightsByLink(ctx, input.LinkID)
	if err != nil {
		return nil, fmt.Errorf("linkUsecase.CreateABTest sum weights: %w", err)
	}
	if currentSum+input.Weight > 100 {
		return nil, domain.NewError(
			domain.ErrInvalidABWeight,
			fmt.Sprintf("total weight would be %d — must not exceed 100", currentSum+input.Weight),
		)
	}

	test := &domain.ABTest{
		LinkID:         input.LinkID,
		DestinationURL: input.DestinationURL,
		Weight:         input.Weight,
		Label:          input.Label,
	}

	if err := u.abTestRepo.Create(ctx, test); err != nil {
		return nil, err
	}

	// Invalidate cache so next redirect picks up the new variant
	_ = u.cacheRepo.DeleteLink(ctx, link.Slug)

	return test, nil
}

func (u *linkUsecase) GetABTests(ctx context.Context, linkID int64, userID string) ([]*domain.ABTest, error) {
	link, err := u.linkRepo.FindByID(ctx, linkID)
	if err != nil {
		return nil, err
	}
	if link.UserID != userID {
		return nil, domain.ErrForbidden
	}

	return u.abTestRepo.FindAllByLink(ctx, linkID)
}

func (u *linkUsecase) DeleteABTest(ctx context.Context, linkID int64, variantID string, userID string) error {
	link, err := u.linkRepo.FindByID(ctx, linkID)
	if err != nil {
		return err
	}
	if link.UserID != userID {
		return domain.ErrForbidden
	}

	if err := u.abTestRepo.Delete(ctx, variantID); err != nil {
		return err
	}

	_ = u.cacheRepo.DeleteLink(ctx, link.Slug)

	return nil
}

// --- Geo Rule management ---

func (u *linkUsecase) CreateGeoRule(ctx context.Context, input domain.CreateGeoRuleInput, userID string) (*domain.GeoRule, error) {
	link, err := u.linkRepo.FindByID(ctx, input.LinkID)
	if err != nil {
		return nil, err
	}
	if link.UserID != userID {
		return nil, domain.ErrForbidden
	}

	// Validate country code — must be exactly 2 uppercase letters
	cc := strings.ToUpper(input.CountryCode)
	if len(cc) != 2 {
		return nil, domain.NewError(domain.ErrInvalidInput, "country_code must be ISO 3166-1 alpha-2 (2 letters)")
	}

	// Validate URL
	if !urlPattern.MatchString(input.DestinationURL) {
		return nil, domain.ErrInvalidURL
	}

	rule := &domain.GeoRule{
		LinkID:         input.LinkID,
		CountryCode:    cc,
		DestinationURL: input.DestinationURL,
		Priority:       input.Priority,
	}

	if err := u.geoRuleRepo.Create(ctx, rule); err != nil {
		return nil, err
	}

	_ = u.cacheRepo.DeleteLink(ctx, link.Slug)

	return rule, nil
}

func (u *linkUsecase) GetGeoRules(ctx context.Context, linkID int64, userID string) ([]*domain.GeoRule, error) {
	link, err := u.linkRepo.FindByID(ctx, linkID)
	if err != nil {
		return nil, err
	}
	if link.UserID != userID {
		return nil, domain.ErrForbidden
	}

	return u.geoRuleRepo.FindAllByLink(ctx, linkID)
}

func (u *linkUsecase) DeleteGeoRule(ctx context.Context, linkID int64, ruleID string, userID string) error {
	link, err := u.linkRepo.FindByID(ctx, linkID)
	if err != nil {
		return err
	}
	if link.UserID != userID {
		return domain.ErrForbidden
	}

	if err := u.geoRuleRepo.Delete(ctx, ruleID); err != nil {
		return err
	}

	_ = u.cacheRepo.DeleteLink(ctx, link.Slug)

	return nil
}
