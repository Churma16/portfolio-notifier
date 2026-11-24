package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TelemetryEvent represents an event payload dispatched to portfolio-monitor.
type TelemetryEvent struct {
	Source     string            `json:"source"`
	Type       string            `json:"type"`
	DurationMs float64           `json:"duration_ms,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

var defaultTelemetryClient = &http.Client{
	Timeout: 100 * time.Millisecond,
}

// dispatchTelemetry asynchronously dispatches telemetry events to portfolio-monitor.
// It is non-blocking, enforces a 100ms bounded HTTP timeout, drains and closes the response body,
// and silently swallows network/connection errors if portfolio-monitor is offline or restarting.
// CRITICAL PRIVACY RULE: Never include PII (Email, Name, Message) in the event or metadata.
func (s *Service) dispatchTelemetry(channel string, eventType string, elapsed time.Duration, backend string, err error) {
	monitorURL := strings.TrimSpace(s.monitorURL)
	if monitorURL == "" {
		return
	}

	backend = strings.TrimSpace(backend)
	if backend == "" {
		backend = "unknown"
	}

	elapsedMs := elapsed.Milliseconds()
	metadata := map[string]string{
		"channel":     channel,
		"duration_ms": strconv.FormatInt(elapsedMs, 10),
		"backend":     backend,
	}
	if err != nil {
		metadata["error"] = err.Error()
	}

	event := TelemetryEvent{
		Source:     "notifier",
		Type:       eventType,
		DurationMs: float64(elapsedMs),
		Metadata:   metadata,
	}

	s.telemetryWg.Add(1)
	go func() {
		defer s.telemetryWg.Done()

		data, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, monitorURL, bytes.NewReader(data))
		if reqErr != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")

		client := s.telemetryClient
		if client == nil {
			client = defaultTelemetryClient
		}

		resp, doErr := client.Do(req)
		if doErr != nil {
			// Silently swallow errors if portfolio-monitor is offline or times out
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
	}()
}
