package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/IndraSty/url-shortener-golang/internal/delivery/http/middleware"
	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/labstack/echo/v4"
)

type AnalyticsHandler struct {
	analyticsUsecase domain.AnalyticsUsecase
}

func NewAnalyticsHandler(analyticsUsecase domain.AnalyticsUsecase) *AnalyticsHandler {
	return &AnalyticsHandler{analyticsUsecase: analyticsUsecase}
}

// GetSummary godoc
// @Summary      Get analytics summary
// @Description  Returns total clicks, unique IPs for a link in a date range
// @Tags         analytics
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  int    true  "Link ID"
// @Param        start_date query string false "Start date RFC3339 (default: 30 days ago)"
// @Param        end_date   query string false "End date RFC3339 (default: now)"
// @Success      200  {object} domain.AnalyticsSummary
// @Failure      403  {object} errorResponse
// @Failure      404  {object} errorResponse
// @Router       /api/v1/links/{id}/analytics [get]
func (h *AnalyticsHandler) GetSummary(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)
	filter := parseAnalyticsFilter(c)

	summary, err := h.analyticsUsecase.GetSummary(c.Request().Context(), id, userID, filter)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, summary)
}

// GetTimeSeries godoc
// @Summary      Get click time series
// @Description  Returns click counts bucketed by hour or day
// @Tags         analytics
// @Produce      json
// @Security     BearerAuth
// @Param        id          path  int    true  "Link ID"
// @Param        granularity query string false "hour or day (default: day)"
// @Param        start_date  query string false "Start date RFC3339"
// @Param        end_date    query string false "End date RFC3339"
// @Success      200  {array} domain.TimeSeriesPoint
// @Router       /api/v1/links/{id}/analytics/timeseries [get]
func (h *AnalyticsHandler) GetTimeSeries(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)
	filter := parseAnalyticsFilter(c)
	filter.Granularity = c.QueryParam("granularity")

	points, err := h.analyticsUsecase.GetTimeSeries(c.Request().Context(), id, userID, filter)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, points)
}

// GetBreakdown godoc
// @Summary      Get analytics breakdown
// @Description  Returns breakdown by country, device, OS, browser, referrer
// @Tags         analytics
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  int    true  "Link ID"
// @Param        start_date query string false "Start date RFC3339"
// @Param        end_date   query string false "End date RFC3339"
// @Param        limit      query int    false "Items per dimension (default: 10, max: 50)"
// @Success      200  {object} domain.AnalyticsBreakdown
// @Router       /api/v1/links/{id}/analytics/breakdown [get]
func (h *AnalyticsHandler) GetBreakdown(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)
	filter := parseAnalyticsFilter(c)

	if l := c.QueryParam("limit"); l != "" {
		if limit, err := strconv.Atoi(l); err == nil {
			filter.Limit = limit
		}
	}

	breakdown, err := h.analyticsUsecase.GetBreakdown(c.Request().Context(), id, userID, filter)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, breakdown)
}

// GetRecentClicks godoc
// @Summary      Get recent click events
// @Description  Returns the most recent N click events for a link
// @Tags         analytics
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int true  "Link ID"
// @Param        limit query int false "Number of events (default: 20, max: 100)"
// @Success      200  {array} domain.ClickEvent
// @Router       /api/v1/links/{id}/analytics/clicks [get]
func (h *AnalyticsHandler) GetRecentClicks(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	limit := 20
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	events, err := h.analyticsUsecase.GetRecentClicks(c.Request().Context(), id, userID, limit)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, events)
}

// parseAnalyticsFilter reads common query params into an AnalyticsFilter.
func parseAnalyticsFilter(c echo.Context) domain.AnalyticsFilter {
	filter := domain.AnalyticsFilter{}

	if s := c.QueryParam("start_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.StartDate = t
		}
	}

	if s := c.QueryParam("end_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.EndDate = t
		}
	}

	return filter
}
