package handler

import (
	"net/http"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/logger"
	"github.com/labstack/echo/v4"
)

// WorkerHandler handles inbound QStash webhook callbacks.
// QStash POSTs the click event payload to /internal/analytics/ingest.
// This endpoint must verify the QStash signature before processing.
type WorkerHandler struct {
	analyticsUsecase domain.AnalyticsUsecase
	qstashCurrentKey string
	qstashNextKey    string
}

func NewWorkerHandler(analyticsUsecase domain.AnalyticsUsecase) *WorkerHandler {
	return &WorkerHandler{analyticsUsecase: analyticsUsecase}
}

// NewWorkerHandlerWithKeys creates a worker handler with QStash signing keys.
func NewWorkerHandlerWithKeys(
	analyticsUsecase domain.AnalyticsUsecase,
	currentKey string,
	nextKey string,
) *WorkerHandler {
	return &WorkerHandler{
		analyticsUsecase: analyticsUsecase,
		qstashCurrentKey: currentKey,
		qstashNextKey:    nextKey,
	}
}

// Ingest godoc
// @Summary      QStash analytics ingest (internal)
// @Description  Receives click event payloads from QStash and persists them
// @Tags         internal
// @Accept       json
// @Produce      json
// @Param        body body domain.ClickEventPayload true "Click event payload"
// @Success      200
// @Failure      400  {object} errorResponse
// @Router       /internal/analytics/ingest [post]
func (h *WorkerHandler) Ingest(c echo.Context) error {
	log := logger.GetLogger(c)

	// Verify QStash signature — reject requests not from QStash
	// QStash signs requests with HMAC-SHA256 using the signing key
	if err := h.verifyQStashSignature(c); err != nil {
		log.Warn().Err(err).Msg("worker: invalid QStash signature")
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
	}

	var payload domain.ClickEventPayload
	if err := c.Bind(&payload); err != nil {
		log.Error().Err(err).Msg("worker: failed to bind payload")
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	// Validate required fields
	if payload.LinkID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "link_id is required")
	}

	if err := h.analyticsUsecase.ProcessClickEvent(c.Request().Context(), payload); err != nil {
		log.Error().Err(err).Int64("link_id", payload.LinkID).Msg("worker: failed to process click event")
		// Return 500 so QStash retries the message (up to 3 times per config)
		return echo.NewHTTPError(http.StatusInternalServerError, "processing failed")
	}

	log.Info().Int64("link_id", payload.LinkID).Msg("worker: click event processed")

	// Return 200 to acknowledge receipt — QStash won't retry on 2xx
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// verifyQStashSignature validates the Upstash-Signature header.
// QStash signs every request — we must reject anything unsigned.
// If no signing key is configured (local dev), skip verification.
func (h *WorkerHandler) verifyQStashSignature(c echo.Context) error {
	if h.qstashCurrentKey == "" {
		// No key configured — skip in development
		return nil
	}

	sig := c.Request().Header.Get("Upstash-Signature")
	if sig == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing Upstash-Signature header")
	}

	// QStash signature verification uses JWT internally.
	// The signature is a JWT signed with the signing key.
	// For production, use the official @upstash/qstash-go SDK or verify manually.
	// We keep this simple: check current key first, then next key (for key rotation).
	if err := verifyJWTSignature(sig, h.qstashCurrentKey); err != nil {
		// Try next key — QStash may have rotated
		if h.qstashNextKey != "" {
			if err2 := verifyJWTSignature(sig, h.qstashNextKey); err2 != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
			}
			return nil
		}
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
	}

	return nil
}

// verifyJWTSignature checks a QStash JWT signature against a signing key.
func verifyJWTSignature(token, signingKey string) error {
	// QStash uses HS256 JWT for signing — same library we already use
	// Minimal verification: just check the signature is valid
	// The JWT body contains the URL and body hash for replay protection
	_, err := parseHMACJWT(token, signingKey)
	return err
}
