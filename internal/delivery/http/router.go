package http

import (
	"net/http"

	"github.com/IndraSty/url-shortener-golang/config"
	_ "github.com/IndraSty/url-shortener-golang/docs"
	"github.com/IndraSty/url-shortener-golang/internal/delivery/http/handler"
	"github.com/IndraSty/url-shortener-golang/internal/delivery/http/middleware"
	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/logger"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	echoswagger "github.com/swaggo/echo-swagger"
)

// RouterDeps holds all dependencies needed to wire the HTTP router.
// Using a struct avoids a long parameter list and makes future additions easy.
type RouterDeps struct {
	Config           *config.Config
	AuthUsecase      domain.AuthUsecase
	LinkUsecase      domain.LinkUsecase
	RedirectUsecase  domain.RedirectUsecase
	AnalyticsUsecase domain.AnalyticsUsecase
	CacheRepo        domain.CacheRepository
	Log              zerolog.Logger
	DB               *pgxpool.Pool
	RedisClient      *redis.Client
}

// NewRouter creates and configures the Echo instance with all routes and middleware.
func NewRouter(deps RouterDeps) *echo.Echo {
	e := echo.New()

	// Disable Echo's default error handler — we use our own
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = customErrorHandler

	// -------------------------------------------------------------------------
	// Global middleware — applied to every request
	// -------------------------------------------------------------------------

	// Request ID — generates X-Request-ID for tracing
	e.Use(echomiddleware.RequestID())

	// Structured request logger — attaches zerolog to Echo context
	e.Use(logger.EchoMiddleware(deps.Log))

	// Security headers on every response
	e.Use(middleware.SecurityHeaders())

	// Prometheus metrics for all requests
	e.Use(middleware.PrometheusMiddleware())

	// Recover from panics — never crash the server on a bug
	e.Use(echomiddleware.Recover())

	// HTTPS redirect — only active in production
	if deps.Config.IsProd() {
		e.Use(middleware.HTTPSRedirect())
	}

	// -------------------------------------------------------------------------
	// Initialize handlers
	// -------------------------------------------------------------------------
	authHandler := handler.NewAuthHandler(deps.AuthUsecase)
	linkHandler := handler.NewLinkHandler(deps.LinkUsecase)
	redirectHandler := handler.NewRedirectHandler(deps.RedirectUsecase)
	analyticsHandler := handler.NewAnalyticsHandler(deps.AnalyticsUsecase)

	// -------------------------------------------------------------------------
	// Public routes — no authentication required
	// -------------------------------------------------------------------------

	// Health handler
	healthHandler := handler.NewHealthHandler(deps.DB, deps.RedisClient)

	// Health check — used by Fly.io and Prometheus
	e.GET("/health", healthCheck)
	e.GET("/livez", healthHandler.Liveness)
	e.GET("/readyz", healthHandler.Readiness)

	// Metrics endpoint — Prometheus scrapes this
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// Swagger UI — disabled in production
	if !deps.Config.IsProd() {
		e.GET("/swagger/*", echoswagger.WrapHandler)
	}

	// Redirect — the hot path, rate limited per IP
	redirectGroup := e.Group("")
	redirectGroup.Use(middleware.RedirectRateLimiter(deps.CacheRepo, 60))
	redirectGroup.Use(middleware.RedirectMetricsMiddleware())
	redirectGroup.GET("/:slug", redirectHandler.Redirect)
	redirectGroup.POST("/:slug/unlock", redirectHandler.UnlockWithPassword)

	// -------------------------------------------------------------------------
	// Auth routes — public, but rate limited to prevent brute force
	// -------------------------------------------------------------------------
	auth := e.Group("/api/v1/auth")
	auth.Use(middleware.APIRateLimiter(deps.CacheRepo, 20)) // strict — 20 req/min
	auth.POST("/register", authHandler.Register)
	auth.POST("/login", authHandler.Login)

	// -------------------------------------------------------------------------
	// Management API — requires authentication (JWT or API key)
	// -------------------------------------------------------------------------
	api := e.Group("/api/v1")
	api.Use(middleware.AnyAuthMiddleware(deps.AuthUsecase))
	api.Use(middleware.APIRateLimiter(deps.CacheRepo, 300))

	// Links CRUD
	api.POST("/links", linkHandler.Create)
	api.GET("/links", linkHandler.GetAll)
	api.GET("/links/:id", linkHandler.GetByID)
	api.PATCH("/links/:id", linkHandler.Update)
	api.DELETE("/links/:id", linkHandler.Delete)
	api.GET("/links/:id/qr", linkHandler.GetQRCode)

	// A/B tests
	api.POST("/links/:id/ab-tests", linkHandler.CreateABTest)
	api.GET("/links/:id/ab-tests", linkHandler.GetABTests)
	api.DELETE("/links/:id/ab-tests/:variantId", linkHandler.DeleteABTest)

	// Geo rules
	api.POST("/links/:id/geo-rules", linkHandler.CreateGeoRule)
	api.GET("/links/:id/geo-rules", linkHandler.GetGeoRules)
	api.DELETE("/links/:id/geo-rules/:ruleId", linkHandler.DeleteGeoRule)

	// Analytics
	api.GET("/links/:id/analytics", analyticsHandler.GetSummary)
	api.GET("/links/:id/analytics/clicks", analyticsHandler.GetRecentClicks)
	api.GET("/links/:id/analytics/breakdown", analyticsHandler.GetBreakdown)
	api.GET("/links/:id/analytics/timeseries", analyticsHandler.GetTimeSeries)

	// -------------------------------------------------------------------------
	// Internal routes — QStash worker callback, not exposed publicly
	// -------------------------------------------------------------------------
	internal := e.Group("/internal")
	internal.POST("/analytics/ingest", handler.NewWorkerHandler(deps.AnalyticsUsecase).Ingest)

	return e
}

// healthCheck returns a simple 200 OK with service status.
// Fly.io polls this every 15 seconds — must respond in < 5s.
func healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "url-shortener",
	})
}

// customErrorHandler maps domain errors to HTTP status codes consistently.
// All error responses follow the same JSON shape: {"error": "message"}.
func customErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	code := http.StatusInternalServerError
	message := "internal server error"

	// Echo HTTP errors (from middleware, validation, etc.)
	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
		if msg, ok := he.Message.(string); ok {
			message = msg
		}
		c.JSON(code, map[string]string{"error": message}) //nolint:errcheck
		return
	}

	// Domain errors — map to HTTP status
	switch {
	case isDomainError(err, domain.ErrLinkNotFound):
		code, message = http.StatusNotFound, "not found"
	case isDomainError(err, domain.ErrUnauthorized):
		code, message = http.StatusUnauthorized, "unauthorized"
	case isDomainError(err, domain.ErrForbidden):
		code, message = http.StatusForbidden, "forbidden"
	case isDomainError(err, domain.ErrPasswordRequired):
		code, message = http.StatusUnauthorized, "password required"
	case isDomainError(err, domain.ErrInvalidPassword):
		code, message = http.StatusUnauthorized, "invalid password"
	case isDomainError(err, domain.ErrSlugAlreadyExists):
		code, message = http.StatusConflict, "slug already exists"
	case isDomainError(err, domain.ErrEmailAlreadyExists):
		code, message = http.StatusConflict, "email already exists"
	case isDomainError(err, domain.ErrInvalidURL):
		code, message = http.StatusBadRequest, "invalid destination URL"
	case isDomainError(err, domain.ErrInvalidSlug):
		code, message = http.StatusBadRequest, "invalid slug format"
	case isDomainError(err, domain.ErrInvalidABWeight):
		code, message = http.StatusBadRequest, err.Error()
	case isDomainError(err, domain.ErrGeoRuleDuplicate):
		code, message = http.StatusConflict, "geo rule already exists for this country"
	case isDomainError(err, domain.ErrRateLimitExceeded):
		code, message = http.StatusTooManyRequests, "rate limit exceeded"
	case isDomainError(err, domain.ErrInvalidCredentials):
		code, message = http.StatusUnauthorized, "invalid credentials"
	case isDomainError(err, domain.ErrInvalidInput):
		code, message = http.StatusBadRequest, err.Error()
	}

	log := logger.GetLogger(c)
	if code == http.StatusInternalServerError {
		log.Error().Err(err).Msg("unhandled error")
	}

	c.JSON(code, map[string]string{"error": message}) //nolint:errcheck
}

// isDomainError checks if err matches a domain sentinel using errors.Is.
func isDomainError(err error, target error) bool {
	// Use standard errors.Is for unwrapping support
	return err != nil && (err == target || isWrapped(err, target))
}

// isWrapped walks the error chain for domain errors.
func isWrapped(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
