package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// errorResponse is the standard error JSON shape for Swagger docs.
type errorResponse struct {
	Error string `json:"error"`
}

// bindAndValidate binds the request body and runs struct validation.
// Returns a 400 HTTPError with a descriptive message on failure.
func bindAndValidate(c echo.Context, req interface{}) error {
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if v, ok := req.(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	return nil
}

// parseLinkID extracts and validates the :id path parameter as int64.
func parseLinkID(c echo.Context) (int64, error) {
	idStr := c.Param("id")
	if idStr == "" {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "missing link id")
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid link id")
	}

	return id, nil
}

// toLinkResponse converts a domain.Link to the API response shape.
func toLinkResponse(l *domain.Link) linkResponse {
	resp := linkResponse{
		ID:             l.ID,
		Slug:           l.Slug,
		DestinationURL: l.DestinationURL,
		Title:          l.Title,
		IsActive:       l.IsActive,
		IsProtected:    l.IsPasswordProtected(),
		ClickCount:     l.ClickCount,
		ExpiredAt:      l.ExpiredAt,
		CreatedAt:      l.CreatedAt,
		UpdatedAt:      l.UpdatedAt,
	}

	// ABTests and GeoRules
	for _, ab := range l.ABTests {
		resp.ABTests = append(resp.ABTests, toABTestResponse(ab))
	}
	for _, gr := range l.GeoRules {
		resp.GeoRules = append(resp.GeoRules, toGeoRuleResponse(gr))
	}

	return resp
}

// toABTestResponse converts a domain.ABTest to API response shape.
func toABTestResponse(t *domain.ABTest) abTestResponse {
	return abTestResponse{
		ID:             t.ID,
		LinkID:         t.LinkID,
		DestinationURL: t.DestinationURL,
		Weight:         t.Weight,
		Label:          t.Label,
		CreatedAt:      t.CreatedAt,
	}
}

// toGeoRuleResponse converts a domain.GeoRule to API response shape.
func toGeoRuleResponse(r *domain.GeoRule) geoRuleResponse {
	return geoRuleResponse{
		ID:             r.ID,
		LinkID:         r.LinkID,
		CountryCode:    r.CountryCode,
		DestinationURL: r.DestinationURL,
		Priority:       r.Priority,
		CreatedAt:      r.CreatedAt,
	}
}

// splitAPIKeyFromToken extracts the raw API key that was appended to the
// access token string during registration.
// Format: "<jwt_token>|apikey:<raw_api_key>"
func splitAPIKeyFromToken(combined string) (token string, apiKey string) {
	const separator = "|apikey:"
	idx := strings.Index(combined, separator)
	if idx == -1 {
		return combined, ""
	}
	return combined[:idx], combined[idx+len(separator):]
}

// parseHMACJWT parses a JWT token signed with HMAC-SHA256.
// Used for QStash signature verification.
func parseHMACJWT(tokenStr, secret string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, echo.NewHTTPError(http.StatusUnauthorized, "unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "invalid claims")
	}

	return claims, nil
}
