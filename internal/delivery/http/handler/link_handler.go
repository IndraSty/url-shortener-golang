package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/IndraSty/url-shortener-golang/internal/delivery/http/middleware"
	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/labstack/echo/v4"
)

type LinkHandler struct {
	linkUsecase domain.LinkUsecase
}

func NewLinkHandler(linkUsecase domain.LinkUsecase) *LinkHandler {
	return &LinkHandler{linkUsecase: linkUsecase}
}

// Create godoc
// @Summary      Create a short link
// @Description  Creates a new shortened URL with optional custom slug, password, expiry
// @Tags         links
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body createLinkRequest true "Link payload"
// @Success      201  {object} linkResponse
// @Failure      400  {object} errorResponse
// @Failure      401  {object} errorResponse
// @Failure      409  {object} errorResponse
// @Router       /api/v1/links [post]
func (h *LinkHandler) Create(c echo.Context) error {
	var req createLinkRequest
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	input := domain.CreateLinkInput{
		UserID:         userID,
		DestinationURL: req.DestinationURL,
		Title:          req.Title,
		CustomSlug:     req.CustomSlug,
		Password:       req.Password,
	}

	if req.ExpiredAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiredAt)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "expired_at must be RFC3339 format")
		}
		input.ExpiredAt = &t
	}

	link, err := h.linkUsecase.Create(c.Request().Context(), input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, toLinkResponse(link))
}

// GetAll godoc
// @Summary      List all links
// @Description  Returns paginated list of links for the authenticated user
// @Tags         links
// @Produce      json
// @Security     BearerAuth
// @Param        limit   query int false "Page size (default 20, max 100)"
// @Param        offset  query int false "Offset for pagination"
// @Success      200  {object} linksListResponse
// @Failure      401  {object} errorResponse
// @Router       /api/v1/links [get]
func (h *LinkHandler) GetAll(c echo.Context) error {
	userID := middleware.GetUserID(c)

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	links, total, err := h.linkUsecase.GetAllByUser(c.Request().Context(), userID, limit, offset)
	if err != nil {
		return err
	}

	items := make([]linkResponse, 0, len(links))
	for _, l := range links {
		items = append(items, toLinkResponse(l))
	}

	return c.JSON(http.StatusOK, linksListResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// GetByID godoc
// @Summary      Get a link by ID
// @Description  Returns a single link with its A/B tests and geo rules
// @Tags         links
// @Produce      json
// @Security     BearerAuth
// @Param        id   path int true "Link ID"
// @Success      200  {object} linkResponse
// @Failure      403  {object} errorResponse
// @Failure      404  {object} errorResponse
// @Router       /api/v1/links/{id} [get]
func (h *LinkHandler) GetByID(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	link, err := h.linkUsecase.GetByID(c.Request().Context(), id, userID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, toLinkResponse(link))
}

// Update godoc
// @Summary      Update a link
// @Description  Partially updates a link — only provided fields are changed
// @Tags         links
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path int true "Link ID"
// @Param        body body updateLinkRequest true "Update payload"
// @Success      200  {object} linkResponse
// @Failure      400  {object} errorResponse
// @Failure      403  {object} errorResponse
// @Failure      404  {object} errorResponse
// @Router       /api/v1/links/{id} [patch]
func (h *LinkHandler) Update(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	var req updateLinkRequest
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	input := domain.UpdateLinkInput{
		DestinationURL: req.DestinationURL,
		Title:          req.Title,
		Password:       req.Password,
		IsActive:       req.IsActive,
	}

	if req.ExpiredAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiredAt)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "expired_at must be RFC3339 format")
		}
		input.ExpiredAt = &t
	}

	link, err := h.linkUsecase.Update(c.Request().Context(), id, userID, input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, toLinkResponse(link))
}

// Delete godoc
// @Summary      Delete a link
// @Description  Soft-deletes a link (sets is_active=false)
// @Tags         links
// @Produce      json
// @Security     BearerAuth
// @Param        id   path int true "Link ID"
// @Success      204
// @Failure      403  {object} errorResponse
// @Failure      404  {object} errorResponse
// @Router       /api/v1/links/{id} [delete]
func (h *LinkHandler) Delete(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	if err := h.linkUsecase.Delete(c.Request().Context(), id, userID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// GetQRCode godoc
// @Summary      Get QR code for a link
// @Description  Returns a PNG QR code image for the short URL
// @Tags         links
// @Produce      image/png
// @Security     BearerAuth
// @Param        id   path int true "Link ID"
// @Success      200  {file} binary
// @Failure      403  {object} errorResponse
// @Failure      404  {object} errorResponse
// @Router       /api/v1/links/{id}/qr [get]
func (h *LinkHandler) GetQRCode(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	png, err := h.linkUsecase.GenerateQRCode(c.Request().Context(), id, userID)
	if err != nil {
		return err
	}

	return c.Blob(http.StatusOK, "image/png", png)
}

// --- A/B Test handlers ---

// CreateABTest godoc
// @Summary      Add an A/B test variant
// @Description  Adds a new destination variant with a traffic weight to a link
// @Tags         ab-tests
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path int true "Link ID"
// @Param        body body createABTestRequest true "Variant payload"
// @Success      201  {object} abTestResponse
// @Failure      400  {object} errorResponse
// @Router       /api/v1/links/{id}/ab-tests [post]
func (h *LinkHandler) CreateABTest(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	var req createABTestRequest
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	test, err := h.linkUsecase.CreateABTest(c.Request().Context(), domain.CreateABTestInput{
		LinkID:         id,
		DestinationURL: req.DestinationURL,
		Weight:         req.Weight,
		Label:          req.Label,
	}, userID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, toABTestResponse(test))
}

// GetABTests godoc
// @Summary      List A/B test variants
// @Description  Returns all variants for a link
// @Tags         ab-tests
// @Produce      json
// @Security     BearerAuth
// @Param        id   path int true "Link ID"
// @Success      200  {array} abTestResponse
// @Router       /api/v1/links/{id}/ab-tests [get]
func (h *LinkHandler) GetABTests(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	tests, err := h.linkUsecase.GetABTests(c.Request().Context(), id, userID)
	if err != nil {
		return err
	}

	items := make([]abTestResponse, 0, len(tests))
	for _, t := range tests {
		items = append(items, toABTestResponse(t))
	}

	return c.JSON(http.StatusOK, items)
}

// DeleteABTest godoc
// @Summary      Delete an A/B test variant
// @Description  Removes a variant from a link's A/B test
// @Tags         ab-tests
// @Produce      json
// @Security     BearerAuth
// @Param        id        path int    true "Link ID"
// @Param        variantId path string true "Variant UUID"
// @Success      204
// @Failure      404  {object} errorResponse
// @Router       /api/v1/links/{id}/ab-tests/{variantId} [delete]
func (h *LinkHandler) DeleteABTest(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	variantID := c.Param("variantId")
	if variantID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "variantId is required")
	}

	userID := middleware.GetUserID(c)

	if err := h.linkUsecase.DeleteABTest(c.Request().Context(), id, variantID, userID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// --- Geo Rule handlers ---

// CreateGeoRule godoc
// @Summary      Add a geo rule
// @Description  Adds a country-specific redirect destination to a link
// @Tags         geo-rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path int true "Link ID"
// @Param        body body createGeoRuleRequest true "Geo rule payload"
// @Success      201  {object} geoRuleResponse
// @Failure      400  {object} errorResponse
// @Failure      409  {object} errorResponse
// @Router       /api/v1/links/{id}/geo-rules [post]
func (h *LinkHandler) CreateGeoRule(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	var req createGeoRuleRequest
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	rule, err := h.linkUsecase.CreateGeoRule(c.Request().Context(), domain.CreateGeoRuleInput{
		LinkID:         id,
		CountryCode:    req.CountryCode,
		DestinationURL: req.DestinationURL,
		Priority:       req.Priority,
	}, userID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, toGeoRuleResponse(rule))
}

// GetGeoRules godoc
// @Summary      List geo rules
// @Description  Returns all geo rules for a link ordered by priority
// @Tags         geo-rules
// @Produce      json
// @Security     BearerAuth
// @Param        id   path int true "Link ID"
// @Success      200  {array} geoRuleResponse
// @Router       /api/v1/links/{id}/geo-rules [get]
func (h *LinkHandler) GetGeoRules(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	userID := middleware.GetUserID(c)

	rules, err := h.linkUsecase.GetGeoRules(c.Request().Context(), id, userID)
	if err != nil {
		return err
	}

	items := make([]geoRuleResponse, 0, len(rules))
	for _, r := range rules {
		items = append(items, toGeoRuleResponse(r))
	}

	return c.JSON(http.StatusOK, items)
}

// DeleteGeoRule godoc
// @Summary      Delete a geo rule
// @Description  Removes a country-specific redirect rule from a link
// @Tags         geo-rules
// @Produce      json
// @Security     BearerAuth
// @Param        id     path int    true "Link ID"
// @Param        ruleId path string true "Geo rule UUID"
// @Success      204
// @Failure      404  {object} errorResponse
// @Router       /api/v1/links/{id}/geo-rules/{ruleId} [delete]
func (h *LinkHandler) DeleteGeoRule(c echo.Context) error {
	id, err := parseLinkID(c)
	if err != nil {
		return err
	}

	ruleID := c.Param("ruleId")
	if ruleID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "ruleId is required")
	}

	userID := middleware.GetUserID(c)

	if err := h.linkUsecase.DeleteGeoRule(c.Request().Context(), id, ruleID, userID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// --- Request / Response types ---

type createLinkRequest struct {
	DestinationURL string `json:"destination_url" validate:"required,url"`
	Title          string `json:"title"`
	CustomSlug     string `json:"custom_slug"`
	Password       string `json:"password"`
	ExpiredAt      string `json:"expired_at"` // RFC3339 string
}

type updateLinkRequest struct {
	DestinationURL *string `json:"destination_url"`
	Title          *string `json:"title"`
	Password       *string `json:"password"`
	IsActive       *bool   `json:"is_active"`
	ExpiredAt      *string `json:"expired_at"` // RFC3339 string
}

type createABTestRequest struct {
	DestinationURL string `json:"destination_url" validate:"required,url"`
	Weight         int    `json:"weight"          validate:"required,min=1,max=100"`
	Label          string `json:"label"           validate:"required"`
}

type createGeoRuleRequest struct {
	CountryCode    string `json:"country_code"    validate:"required,len=2"`
	DestinationURL string `json:"destination_url" validate:"required,url"`
	Priority       int    `json:"priority"`
}

type linkResponse struct {
	ID             int64             `json:"id"`
	Slug           string            `json:"slug"`
	ShortURL       string            `json:"short_url"`
	DestinationURL string            `json:"destination_url"`
	Title          string            `json:"title"`
	IsActive       bool              `json:"is_active"`
	IsProtected    bool              `json:"is_protected"`
	ClickCount     int64             `json:"click_count"`
	ExpiredAt      *time.Time        `json:"expired_at"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	ABTests        []abTestResponse  `json:"ab_tests,omitempty"`
	GeoRules       []geoRuleResponse `json:"geo_rules,omitempty"`
}

type linksListResponse struct {
	Items  []linkResponse `json:"items"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type abTestResponse struct {
	ID             string    `json:"id"`
	LinkID         int64     `json:"link_id"`
	DestinationURL string    `json:"destination_url"`
	Weight         int       `json:"weight"`
	Label          string    `json:"label"`
	CreatedAt      time.Time `json:"created_at"`
}

type geoRuleResponse struct {
	ID             string    `json:"id"`
	LinkID         int64     `json:"link_id"`
	CountryCode    string    `json:"country_code"`
	DestinationURL string    `json:"destination_url"`
	Priority       int       `json:"priority"`
	CreatedAt      time.Time `json:"created_at"`
}
