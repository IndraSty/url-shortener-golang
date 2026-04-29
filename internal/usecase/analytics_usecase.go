package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/geoip"
	"github.com/IndraSty/url-shortener-golang/pkg/useragent"
	"github.com/google/uuid"
)

type analyticsUsecase struct {
	analyticsRepo domain.AnalyticsRepository
	linkRepo      domain.LinkRepository
	geoClient     *geoip.Client
}

// NewAnalyticsUsecase creates the analytics usecase.
func NewAnalyticsUsecase(
	analyticsRepo domain.AnalyticsRepository,
	linkRepo domain.LinkRepository,
	geoClient *geoip.Client,
) domain.AnalyticsUsecase {
	return &analyticsUsecase{
		analyticsRepo: analyticsRepo,
		linkRepo:      linkRepo,
		geoClient:     geoClient,
	}
}

func (u *analyticsUsecase) GetSummary(
	ctx context.Context,
	linkID int64,
	userID string,
	filter domain.AnalyticsFilter,
) (*domain.AnalyticsSummary, error) {
	if err := u.authorizeLink(ctx, linkID, userID); err != nil {
		return nil, err
	}

	filter.LinkID = linkID
	u.applyDefaultFilter(&filter)

	return u.analyticsRepo.GetSummary(ctx, filter)
}

func (u *analyticsUsecase) GetTimeSeries(
	ctx context.Context,
	linkID int64,
	userID string,
	filter domain.AnalyticsFilter,
) ([]domain.TimeSeriesPoint, error) {
	if err := u.authorizeLink(ctx, linkID, userID); err != nil {
		return nil, err
	}

	filter.LinkID = linkID
	u.applyDefaultFilter(&filter)

	// Validate granularity
	if filter.Granularity != "hour" && filter.Granularity != "day" {
		filter.Granularity = "day"
	}

	return u.analyticsRepo.GetTimeSeries(ctx, filter)
}

func (u *analyticsUsecase) GetBreakdown(
	ctx context.Context,
	linkID int64,
	userID string,
	filter domain.AnalyticsFilter,
) (*domain.AnalyticsBreakdown, error) {
	if err := u.authorizeLink(ctx, linkID, userID); err != nil {
		return nil, err
	}

	filter.LinkID = linkID
	u.applyDefaultFilter(&filter)

	if filter.Limit <= 0 || filter.Limit > 50 {
		filter.Limit = 10
	}

	return u.analyticsRepo.GetBreakdown(ctx, filter)
}

func (u *analyticsUsecase) GetRecentClicks(
	ctx context.Context,
	linkID int64,
	userID string,
	limit int,
) ([]*domain.ClickEvent, error) {
	if err := u.authorizeLink(ctx, linkID, userID); err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	return u.analyticsRepo.GetRecentClicks(ctx, linkID, limit)
}

// ProcessClickEvent is called by the worker after receiving a QStash message.
// It enriches the raw payload (geo lookup, UA parse, IP mask) and persists it.
func (u *analyticsUsecase) ProcessClickEvent(ctx context.Context, payload domain.ClickEventPayload) error {
	// Resolve geo location from raw IP
	loc := u.geoClient.Lookup(ctx, payload.IPAddress)

	// Parse user agent
	uaInfo := useragent.Parse(payload.UserAgent)

	// Strip referrer to domain only
	referrer := useragent.StripReferrer(payload.Referrer)

	// Mask IP for GDPR compliance — never store full IP
	maskedIP := geoip.MaskIP(payload.IPAddress)

	// Parse clicked_at from payload
	clickedAt, err := time.Parse(time.RFC3339, payload.ClickedAt)
	if err != nil {
		clickedAt = time.Now() // fallback
	}

	event := &domain.ClickEvent{
		ID:          uuid.New().String(),
		LinkID:      payload.LinkID,
		ABTestID:    payload.ABTestID,
		IPAddress:   maskedIP,
		CountryCode: loc.CountryCode,
		City:        loc.City,
		Device:      uaInfo.Device,
		OS:          uaInfo.OS,
		Browser:     uaInfo.Browser,
		Referrer:    referrer,
		UserAgent:   payload.UserAgent,
		ClickedAt:   clickedAt,
	}

	if err := u.analyticsRepo.SaveClickEvent(ctx, event); err != nil {
		return fmt.Errorf("analyticsUsecase.ProcessClickEvent save: %w", err)
	}

	// Increment the denormalized click counter on the link
	if err := u.linkRepo.IncrementClickCount(ctx, payload.LinkID); err != nil {
		// Non-fatal — counter is denormalized, authoritative count is in click_events
		_ = err
	}

	return nil
}

// --- Internal helpers ---

// authorizeLink verifies that linkID belongs to userID.
func (u *analyticsUsecase) authorizeLink(ctx context.Context, linkID int64, userID string) error {
	link, err := u.linkRepo.FindByID(ctx, linkID)
	if err != nil {
		return err
	}
	if link.UserID != userID {
		return domain.ErrForbidden
	}
	return nil
}

// applyDefaultFilter sets sensible defaults when filter fields are zero values.
func (u *analyticsUsecase) applyDefaultFilter(f *domain.AnalyticsFilter) {
	if f.EndDate.IsZero() {
		f.EndDate = time.Now()
	}
	if f.StartDate.IsZero() {
		// Default to last 30 days
		f.StartDate = f.EndDate.AddDate(0, 0, -30)
	}
	if f.Granularity == "" {
		f.Granularity = "day"
	}
}
