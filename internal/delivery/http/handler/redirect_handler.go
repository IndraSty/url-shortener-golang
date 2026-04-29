package handler

import (
	"net/http"
	"time"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/logger"
	"github.com/labstack/echo/v4"
)

type RedirectHandler struct {
	redirectUsecase domain.RedirectUsecase
}

func NewRedirectHandler(redirectUsecase domain.RedirectUsecase) *RedirectHandler {
	return &RedirectHandler{redirectUsecase: redirectUsecase}
}

// Redirect godoc
// @Summary      Redirect to destination
// @Description  Resolves a short slug and redirects — target sub-10ms latency
// @Tags         public
// @Param        slug path string true "Short slug"
// @Success      301
// @Success      302
// @Failure      401  {object} errorResponse "Password required"
// @Failure      404  {object} errorResponse
// @Failure      429  {object} errorResponse
// @Router       /{slug} [get]
func (h *RedirectHandler) Redirect(c echo.Context) error {
	slug := c.Param("slug")
	if slug == "" {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	log := logger.GetLogger(c)

	input := domain.RedirectInput{
		Slug:      slug,
		IP:        c.RealIP(),
		UserAgent: c.Request().Header.Get("User-Agent"),
		Referrer:  c.Request().Header.Get("Referer"),
	}

	result, err := h.redirectUsecase.Redirect(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Publish click event asynchronously — AFTER redirect decision is made.
	// Use a goroutine so QStash HTTP call never delays the redirect response.
	go func() {
		payload := domain.ClickEventPayload{
			LinkID:    extractLinkIDFromResult(result),
			ABTestID:  result.ABTestID,
			IPAddress: input.IP,
			UserAgent: input.UserAgent,
			Referrer:  input.Referrer,
			ClickedAt: time.Now().UTC().Format(time.RFC3339),
		}

		ctx := c.Request().Context()
		if err := h.redirectUsecase.PublishClickEvent(ctx, payload); err != nil {
			log.Error().Err(err).Str("slug", slug).Msg("failed to publish click event")
		}
	}()

	return c.Redirect(result.StatusCode, result.DestinationURL)
}

// UnlockWithPassword godoc
// @Summary      Unlock a password-protected link
// @Description  Submits password for a protected link and returns the destination URL
// @Tags         public
// @Accept       json
// @Produce      json
// @Param        slug path string true "Short slug"
// @Param        body body unlockRequest true "Password payload"
// @Success      200  {object} unlockResponse
// @Failure      401  {object} errorResponse
// @Failure      404  {object} errorResponse
// @Router       /{slug}/unlock [post]
func (h *RedirectHandler) UnlockWithPassword(c echo.Context) error {
	slug := c.Param("slug")
	if slug == "" {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	var req unlockRequest
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}

	result, err := h.redirectUsecase.UnlockWithPassword(
		c.Request().Context(),
		slug,
		req.Password,
	)
	if err != nil {
		return err
	}

	// Return destination URL — client handles the redirect
	// (useful for SPA frontends that need to show a transition)
	return c.JSON(http.StatusOK, unlockResponse{
		DestinationURL: result.DestinationURL,
	})
}

// --- Request / Response types ---

type unlockRequest struct {
	Password string `json:"password" validate:"required"`
}

type unlockResponse struct {
	DestinationURL string `json:"destination_url"`
}

// extractLinkIDFromResult is a placeholder — the redirect result doesn't carry
// link ID directly. We fix this by adding LinkID to RedirectResult in domain.
// See patch below.
func extractLinkIDFromResult(result *domain.RedirectResult) int64 {
	return result.LinkID
}
