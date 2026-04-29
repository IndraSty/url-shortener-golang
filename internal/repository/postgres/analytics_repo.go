package postgres

import (
	"context"
	"fmt"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type analyticsRepository struct {
	db *pgxpool.Pool
}

func NewAnalyticsRepository(db *pgxpool.Pool) domain.AnalyticsRepository {
	return &analyticsRepository{db: db}
}

func (r *analyticsRepository) SaveClickEvent(ctx context.Context, event *domain.ClickEvent) error {
	query := `
		INSERT INTO click_events
			(link_id, ab_test_id, ip_address, country_code, city,
			 device, os, browser, referrer, user_agent, clicked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	// Convert empty string ab_test_id to nil for nullable FK
	var abTestID *string
	if event.ABTestID != "" {
		abTestID = &event.ABTestID
	}

	_, err := r.db.Exec(ctx, query,
		event.LinkID,
		abTestID,
		event.IPAddress,
		event.CountryCode,
		event.City,
		event.Device,
		event.OS,
		event.Browser,
		event.Referrer,
		event.UserAgent,
		event.ClickedAt,
	)
	if err != nil {
		return fmt.Errorf("analyticsRepository.SaveClickEvent: %w", err)
	}
	return nil
}

func (r *analyticsRepository) GetSummary(ctx context.Context, filter domain.AnalyticsFilter) (*domain.AnalyticsSummary, error) {
	query := `
		SELECT
			COUNT(*)                        AS total_clicks,
			COUNT(DISTINCT ip_address)      AS unique_ips
		FROM click_events
		WHERE link_id = $1
		  AND clicked_at BETWEEN $2 AND $3`

	summary := &domain.AnalyticsSummary{LinkID: filter.LinkID}
	err := r.db.QueryRow(ctx, query,
		filter.LinkID,
		filter.StartDate,
		filter.EndDate,
	).Scan(&summary.TotalClicks, &summary.UniqueIPs)

	if err != nil {
		return nil, fmt.Errorf("analyticsRepository.GetSummary: %w", err)
	}
	return summary, nil
}

func (r *analyticsRepository) GetTimeSeries(ctx context.Context, filter domain.AnalyticsFilter) ([]domain.TimeSeriesPoint, error) {
	// Use date_trunc for bucketing — supports both 'hour' and 'day'
	query := `
		SELECT
			date_trunc($1, clicked_at) AS bucket,
			COUNT(*)                    AS clicks
		FROM click_events
		WHERE link_id = $2
		  AND clicked_at BETWEEN $3 AND $4
		GROUP BY bucket
		ORDER BY bucket ASC`

	rows, err := r.db.Query(ctx, query,
		filter.Granularity,
		filter.LinkID,
		filter.StartDate,
		filter.EndDate,
	)
	if err != nil {
		return nil, fmt.Errorf("analyticsRepository.GetTimeSeries: %w", err)
	}
	defer rows.Close()

	var points []domain.TimeSeriesPoint
	for rows.Next() {
		var p domain.TimeSeriesPoint
		if err := rows.Scan(&p.Bucket, &p.Clicks); err != nil {
			return nil, fmt.Errorf("analyticsRepository.GetTimeSeries scan: %w", err)
		}
		points = append(points, p)
	}

	return points, rows.Err()
}

func (r *analyticsRepository) GetBreakdown(ctx context.Context, filter domain.AnalyticsFilter) (*domain.AnalyticsBreakdown, error) {
	// Run all 5 breakdown queries — each is fast due to composite indexes
	breakdown := &domain.AnalyticsBreakdown{}
	var err error

	breakdown.Countries, err = r.getBreakdownByField(ctx, filter, "country_code")
	if err != nil {
		return nil, err
	}

	breakdown.Devices, err = r.getBreakdownByField(ctx, filter, "device")
	if err != nil {
		return nil, err
	}

	breakdown.OSes, err = r.getBreakdownByField(ctx, filter, "os")
	if err != nil {
		return nil, err
	}

	breakdown.Browsers, err = r.getBreakdownByField(ctx, filter, "browser")
	if err != nil {
		return nil, err
	}

	breakdown.Referrers, err = r.getBreakdownByField(ctx, filter, "referrer")
	if err != nil {
		return nil, err
	}

	return breakdown, nil
}

// getBreakdownByField is a generic helper for all breakdown queries.
// The field name is not user input — it's always called with a hardcoded
// column name from GetBreakdown above, so it's safe from SQL injection.
func (r *analyticsRepository) getBreakdownByField(
	ctx context.Context,
	filter domain.AnalyticsFilter,
	field string,
) ([]domain.BreakdownItem, error) {
	// field is always a hardcoded column name, never user-provided input
	query := fmt.Sprintf(`
		SELECT
			COALESCE(NULLIF(%s, ''), 'unknown') AS label,
			COUNT(*)                             AS clicks
		FROM click_events
		WHERE link_id = $1
		  AND clicked_at BETWEEN $2 AND $3
		GROUP BY label
		ORDER BY clicks DESC
		LIMIT $4`, field)

	limit := filter.Limit
	if limit == 0 {
		limit = 10
	}

	rows, err := r.db.Query(ctx, query, filter.LinkID, filter.StartDate, filter.EndDate, limit)
	if err != nil {
		return nil, fmt.Errorf("analyticsRepository.getBreakdownByField(%s): %w", field, err)
	}
	defer rows.Close()

	var items []domain.BreakdownItem
	var total int64

	for rows.Next() {
		var item domain.BreakdownItem
		if err := rows.Scan(&item.Label, &item.Clicks); err != nil {
			return nil, fmt.Errorf("analyticsRepository.getBreakdownByField scan: %w", err)
		}
		total += item.Clicks
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Calculate percentage for each item
	if total > 0 {
		for i := range items {
			items[i].Percentage = float64(items[i].Clicks) / float64(total) * 100
		}
	}

	return items, nil
}

func (r *analyticsRepository) GetRecentClicks(ctx context.Context, linkID int64, limit int) ([]*domain.ClickEvent, error) {
	query := `
		SELECT id, link_id, COALESCE(ab_test_id::text, ''),
		       ip_address, country_code, city, device, os,
		       browser, referrer, user_agent, clicked_at
		FROM click_events
		WHERE link_id = $1
		ORDER BY clicked_at DESC
		LIMIT $2`

	rows, err := r.db.Query(ctx, query, linkID, limit)
	if err != nil {
		return nil, fmt.Errorf("analyticsRepository.GetRecentClicks: %w", err)
	}
	defer rows.Close()

	var events []*domain.ClickEvent
	for rows.Next() {
		e := &domain.ClickEvent{}
		if err := rows.Scan(
			&e.ID, &e.LinkID, &e.ABTestID,
			&e.IPAddress, &e.CountryCode, &e.City,
			&e.Device, &e.OS, &e.Browser,
			&e.Referrer, &e.UserAgent, &e.ClickedAt,
		); err != nil {
			return nil, fmt.Errorf("analyticsRepository.GetRecentClicks scan: %w", err)
		}
		events = append(events, e)
	}

	return events, rows.Err()
}
