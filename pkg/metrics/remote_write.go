package metrics

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/rs/zerolog"
)

// StartRemoteWrite periodically pushes metrics to Grafana Cloud.
// Runs every 15 seconds — same interval as Prometheus default scrape.
func StartRemoteWrite(
	ctx context.Context,
	remoteWriteURL string,
	username string,
	apiKey string,
	log zerolog.Logger,
) {
	if remoteWriteURL == "" {
		log.Info().Msg("grafana remote write not configured — skipping")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	log.Info().Str("url", remoteWriteURL).Msg("grafana remote write started")

	for {
		select {
		case <-ticker.C:
			if err := pushMetrics(ctx, client, remoteWriteURL, username, apiKey); err != nil {
				log.Error().Err(err).Msg("grafana remote write failed")
			}
		case <-ctx.Done():
			log.Info().Msg("grafana remote write stopped")
			return
		}
	}
}

func pushMetrics(
	ctx context.Context,
	client *http.Client,
	url, username, apiKey string,
) error {
	// Gather all metrics
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return fmt.Errorf("gather metrics: %w", err)
	}

	// Encode as Prometheus text format
	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			return fmt.Errorf("encode metrics: %w", err)
		}
	}

	// Push to Grafana Cloud
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.SetBasicAuth(username, apiKey)
	req.Header.Set("Content-Type", string(expfmt.NewFormat(expfmt.TypeTextPlain)))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("push request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("grafana returned status %d", resp.StatusCode)
	}

	return nil
}
