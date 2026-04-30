package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

// HealthHandler performs deep health checks on all critical dependencies.
// Fly.io polls /health every 15s — must respond in < 5s.
type HealthHandler struct {
	db          *pgxpool.Pool
	redisClient *redis.Client
	startTime   time.Time
}

// NewHealthHandler creates a health check handler.
func NewHealthHandler(db *pgxpool.Pool, redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{
		db:          db,
		redisClient: redisClient,
		startTime:   time.Now(),
	}
}

// healthStatus represents the status of a single dependency.
type healthStatus struct {
	Status  string `json:"status"`            // "ok" | "degraded" | "down"
	Latency string `json:"latency,omitempty"` // e.g. "2ms"
	Error   string `json:"error,omitempty"`
}

// healthResponse is the full health check response body.
type healthResponse struct {
	Status  string                  `json:"status"` // "ok" | "degraded"
	Uptime  string                  `json:"uptime"`
	Checks  map[string]healthStatus `json:"checks"`
	Version string                  `json:"version"`
}

// Check godoc
// @Summary      Health check
// @Description  Returns service health with dependency status (DB + Redis)
// @Tags         system
// @Produce      json
// @Success      200 {object} healthResponse
// @Failure      503 {object} healthResponse
// @Router       /health [get]
func (h *HealthHandler) Check(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 4*time.Second)
	defer cancel()

	checks := make(map[string]healthStatus)
	overallOK := true

	// --- PostgreSQL check ---
	pgStart := time.Now()
	pgErr := h.db.Ping(ctx)
	pgLatency := time.Since(pgStart)

	if pgErr != nil {
		overallOK = false
		checks["postgres"] = healthStatus{
			Status: "down",
			Error:  pgErr.Error(),
		}
	} else {
		checks["postgres"] = healthStatus{
			Status:  "ok",
			Latency: pgLatency.Round(time.Millisecond).String(),
		}
	}

	// --- Redis check ---
	redisStart := time.Now()
	redisErr := h.redisClient.Ping(ctx).Err()
	redisLatency := time.Since(redisStart)

	if redisErr != nil {
		// Redis down = degraded not down — we can serve from DB
		checks["redis"] = healthStatus{
			Status: "degraded",
			Error:  redisErr.Error(),
		}
		// Don't set overallOK = false — service still works without Redis
	} else {
		checks["redis"] = healthStatus{
			Status:  "ok",
			Latency: redisLatency.Round(time.Millisecond).String(),
		}
	}

	// --- Self check ---
	checks["api"] = healthStatus{
		Status: "ok",
	}

	overallStatus := "ok"
	httpStatus := http.StatusOK

	if !overallOK {
		overallStatus = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	return c.JSON(httpStatus, healthResponse{
		Status:  overallStatus,
		Uptime:  time.Since(h.startTime).Round(time.Second).String(),
		Checks:  checks,
		Version: "1.0.0",
	})
}

// Liveness is a minimal liveness probe — just returns 200.
// Fly.io uses this to know if the process is alive (vs. the full health check).
// A failing liveness probe causes Fly.io to restart the machine.
func (h *HealthHandler) Liveness(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "alive"})
}

// Readiness checks if the service is ready to accept traffic.
// Both DB and Redis must be reachable before we accept requests.
// Used during rolling deploys — Fly.io won't route traffic until this passes.
func (h *HealthHandler) Readiness(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"reason": "postgres unavailable",
		})
	}

	if err := h.redisClient.Ping(ctx).Err(); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"reason": "redis unavailable",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}
