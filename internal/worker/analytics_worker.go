package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/rs/zerolog"
)

// AnalyticsWorker is an optional standalone worker that polls a queue.
// In production we rely on QStash push (via /internal/analytics/ingest),
// so this worker is used for local development when QStash is not configured.
//
// Architecture note:
// Production flow:  Redirect → QStash publish → QStash POSTs to /internal/analytics/ingest → WorkerHandler
// Local dev flow:   Redirect → in-memory channel → AnalyticsWorker.Start()
type AnalyticsWorker struct {
	analyticsUsecase domain.AnalyticsUsecase
	queue            chan domain.ClickEventPayload
	log              zerolog.Logger
	concurrency      int
}

// NewAnalyticsWorker creates an analytics worker backed by an in-memory channel.
// bufferSize controls how many events can queue up before Redirect blocks —
// set it large enough that a slow DB never affects redirect latency.
func NewAnalyticsWorker(
	analyticsUsecase domain.AnalyticsUsecase,
	log zerolog.Logger,
	bufferSize int,
	concurrency int,
) *AnalyticsWorker {
	if bufferSize <= 0 {
		bufferSize = 10000
	}
	if concurrency <= 0 {
		concurrency = 4
	}

	return &AnalyticsWorker{
		analyticsUsecase: analyticsUsecase,
		queue:            make(chan domain.ClickEventPayload, bufferSize),
		log:              log.With().Str("component", "analytics_worker").Logger(),
		concurrency:      concurrency,
	}
}

// Enqueue adds a click event to the in-memory queue.
// Non-blocking — if the queue is full, the event is dropped (redirect wins).
// Used only in local dev; production uses QStash.
func (w *AnalyticsWorker) Enqueue(payload domain.ClickEventPayload) {
	select {
	case w.queue <- payload:
		// queued successfully
	default:
		// Queue full — drop event rather than blocking the redirect goroutine.
		// This is an acceptable trade-off: redirect latency > analytics completeness.
		w.log.Warn().
			Int64("link_id", payload.LinkID).
			Msg("analytics queue full — event dropped")
	}
}

// Start launches N worker goroutines that drain the queue.
// Blocks until ctx is canceled, then drains remaining events with a 5s timeout.
func (w *AnalyticsWorker) Start(ctx context.Context) {
	w.log.Info().
		Int("concurrency", w.concurrency).
		Msg("analytics worker started")

	// Launch N consumer goroutines
	done := make(chan struct{}, w.concurrency)
	for i := 0; i < w.concurrency; i++ {
		go func(workerID int) {
			defer func() { done <- struct{}{} }()
			w.consume(ctx, workerID)
		}(i)
	}

	// Wait for context cancellation
	<-ctx.Done()
	w.log.Info().Msg("analytics worker shutting down — draining queue")

	// Drain remaining events with a 5s deadline
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Signal consumers to stop after drain
	close(w.queue)

	// Wait for all consumers to finish
	drainDone := make(chan struct{})
	go func() {
		for i := 0; i < w.concurrency; i++ {
			<-done
		}
		close(drainDone)
	}()

	select {
	case <-drainDone:
		w.log.Info().Msg("analytics worker drained cleanly")
	case <-drainCtx.Done():
		w.log.Warn().Msg("analytics worker drain timed out — some events may be lost")
	}
}

// consume reads from the queue and processes events until ctx is canceled
// or the queue is closed.
func (w *AnalyticsWorker) consume(ctx context.Context, workerID int) {
	log := w.log.With().Int("worker_id", workerID).Logger()

	for payload := range w.queue {
		// Give each event a generous processing window
		processCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		if err := w.analyticsUsecase.ProcessClickEvent(processCtx, payload); err != nil {
			log.Error().
				Err(err).
				Int64("link_id", payload.LinkID).
				Msg("failed to process click event")
		}

		cancel()

		// Check if parent context was canceled between events
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// QueueDepth returns the current number of pending events in the queue.
// Useful for metrics and health checks.
func (w *AnalyticsWorker) QueueDepth() int {
	return len(w.queue)
}

// --- HTTP health probe for the worker ---

// ServeHTTP implements a minimal health endpoint for the worker process.
// Reports queue depth so monitoring can alert on backpressure.
func (w *AnalyticsWorker) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)

	resp := map[string]interface{}{
		"status":      "ok",
		"queue_depth": w.QueueDepth(),
	}

	data, _ := json.Marshal(resp)
	rw.Write(data) //nolint:errcheck
}

// Validate checks that the worker is properly configured.
func (w *AnalyticsWorker) Validate() error {
	if w.analyticsUsecase == nil {
		return fmt.Errorf("analyticsUsecase is required")
	}
	if w.queue == nil {
		return fmt.Errorf("queue is not initialized")
	}
	return nil
}

// logEvent is a helper that logs a processed event at debug level.
func (w *AnalyticsWorker) logEvent(log zerolog.Logger, payload domain.ClickEventPayload) {
	log.Debug().
		Int64("link_id", payload.LinkID).
		Str("ip", payload.IPAddress).
		Str("ua", payload.UserAgent).
		Msg("click event processed")
}
