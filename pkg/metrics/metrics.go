package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// All metrics are registered via promauto — auto-registered on package init,
// no manual Register() calls needed.

var (
	// -------------------------------------------------------------------------
	// HTTP metrics
	// -------------------------------------------------------------------------

	// HTTPRequestsTotal counts all HTTP requests by method, path, and status.
	// Use rate(http_requests_total[5m]) in Grafana for RPS dashboards.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration tracks latency distribution per endpoint.
	// Buckets are tuned for a URL shortener — most requests < 10ms.
	// Use histogram_quantile(0.99, ...) for p99 latency in Grafana.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "HTTP request latency in seconds",
			// Fine-grained buckets under 100ms — this is where we care most
			Buckets: []float64{
				0.001, // 1ms
				0.005, // 5ms
				0.010, // 10ms  ← our SLO target for redirects
				0.025, // 25ms
				0.050, // 50ms
				0.100, // 100ms
				0.250, // 250ms
				0.500, // 500ms
				1.000, // 1s
			},
		},
		[]string{"method", "path"},
	)

	// HTTPResponseSize tracks response body size distribution.
	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 2, 10),
		},
		[]string{"method", "path"},
	)

	// -------------------------------------------------------------------------
	// Redirect-specific metrics
	// -------------------------------------------------------------------------

	// RedirectsTotal counts redirects by outcome.
	// Labels: result = "hit_cache" | "hit_db" | "not_found" | "expired" | "password_required"
	RedirectsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redirects_total",
			Help: "Total redirect attempts by result",
		},
		[]string{"result"},
	)

	// RedirectLatency is a dedicated histogram for redirect latency only.
	// Separate from the general HTTP histogram so we can set a tighter SLO alert.
	RedirectLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "redirect_latency_seconds",
			Help: "Redirect resolution latency (cache lookup + decision logic)",
			Buckets: []float64{
				0.001, 0.002, 0.003, 0.005,
				0.007, 0.010, 0.015, 0.020,
				0.050, 0.100,
			},
		},
	)

	// CacheHitsTotal counts Redis cache hits for redirect lookups.
	CacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total Redis cache hits for link lookups",
	})

	// CacheMissesTotal counts Redis cache misses (fell back to PostgreSQL).
	CacheMissesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "Total Redis cache misses for link lookups",
	})

	// -------------------------------------------------------------------------
	// Analytics worker metrics
	// -------------------------------------------------------------------------

	// ClickEventsProcessed counts events successfully written to DB.
	ClickEventsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "click_events_processed_total",
		Help: "Total click events successfully persisted",
	})

	// ClickEventsDropped counts events dropped due to queue overflow.
	ClickEventsDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "click_events_dropped_total",
		Help: "Total click events dropped due to queue overflow",
	})

	// AnalyticsQueueDepth is a gauge tracking current queue backlog.
	// Alert if this stays high — means DB is too slow to keep up.
	AnalyticsQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "analytics_queue_depth",
		Help: "Current number of unprocessed click events in queue",
	})

	// ClickEventProcessingDuration tracks how long DB inserts take.
	ClickEventProcessingDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "click_event_processing_duration_seconds",
			Help:    "Time to process and persist a single click event",
			Buckets: prometheus.DefBuckets,
		},
	)

	// -------------------------------------------------------------------------
	// Database metrics
	// -------------------------------------------------------------------------

	// DBQueryDuration tracks PostgreSQL query latency by operation.
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "PostgreSQL query latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// DBErrorsTotal counts DB errors by operation type.
	DBErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "db_errors_total",
			Help: "Total PostgreSQL errors by operation",
		},
		[]string{"operation"},
	)

	// -------------------------------------------------------------------------
	// Business metrics
	// -------------------------------------------------------------------------

	// LinksCreatedTotal counts new links created.
	LinksCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "links_created_total",
		Help: "Total short links created",
	})

	// ActiveLinksGauge is set periodically — tracks total active links.
	ActiveLinksGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "active_links_total",
		Help: "Total number of active links",
	})

	// UsersRegisteredTotal counts new user registrations.
	UsersRegisteredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "users_registered_total",
		Help: "Total user registrations",
	})

	// RateLimitHitsTotal counts rate limit rejections by endpoint type.
	RateLimitHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_hits_total",
			Help: "Total requests rejected by rate limiter",
		},
		[]string{"endpoint_type"}, // "redirect" | "api" | "auth"
	)
)
