package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockEmailSender struct {
	sendFunc func(ctx context.Context, req NotificationRequest) error
}

func (m *mockEmailSender) SendEmail(ctx context.Context, req NotificationRequest) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, req)
	}
	return nil
}

type mockTelegramSender struct {
	sendFunc func(ctx context.Context, req NotificationRequest) error
}

func (m *mockTelegramSender) SendTelegram(ctx context.Context, req NotificationRequest) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, req)
	}
	return nil
}

func TestService_DispatchAsync(t *testing.T) {
	var emailCalled int32
	var telegramCalled int32

	emailMock := &mockEmailSender{
		sendFunc: func(ctx context.Context, req NotificationRequest) error {
			atomic.AddInt32(&emailCalled, 1)
			return nil
		},
	}

	telegramMock := &mockTelegramSender{
		sendFunc: func(ctx context.Context, req NotificationRequest) error {
			atomic.AddInt32(&telegramCalled, 1)
			return nil
		},
	}

	svc := NewService(emailMock, telegramMock, 2*time.Second)

	req := NotificationRequest{
		Name:    "Tester",
		Email:   "tester@example.com",
		Message: "Dispatch test",
		Source:  "express",
	}

	svc.DispatchAsync(req)

	// Wait up to 1 second for async goroutines to complete
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&emailCalled) == 1 && atomic.LoadInt32(&telegramCalled) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if atomic.LoadInt32(&emailCalled) != 1 {
		t.Errorf("expected email to be called 1 time, got %d", atomic.LoadInt32(&emailCalled))
	}
	if atomic.LoadInt32(&telegramCalled) != 1 {
		t.Errorf("expected telegram to be called 1 time, got %d", atomic.LoadInt32(&telegramCalled))
	}
}

func TestService_DispatchAsync_HandlesErrorsGracefully(t *testing.T) {
	emailMock := &mockEmailSender{
		sendFunc: func(ctx context.Context, req NotificationRequest) error {
			return errors.New("simulated email outage")
		},
	}

	telegramMock := &mockTelegramSender{
		sendFunc: func(ctx context.Context, req NotificationRequest) error {
			return errors.New("simulated telegram outage")
		},
	}

	svc := NewService(emailMock, telegramMock, 500*time.Millisecond)

	// Dispatch should not panic or block
	svc.DispatchAsync(NotificationRequest{
		Name:    "Tester",
		Email:   "tester@example.com",
		Message: "Outage test",
	})

	time.Sleep(100 * time.Millisecond)
}

func TestService_Wait(t *testing.T) {
	var executed int32

	emailMock := &mockEmailSender{
		sendFunc: func(ctx context.Context, req NotificationRequest) error {
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&executed, 1)
			return nil
		},
	}

	svc := NewService(emailMock, nil, time.Second)
	svc.DispatchAsync(NotificationRequest{Name: "WaitTest", Email: "w@w.com", Message: "hello"})

	svc.Wait(500 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 1 {
		t.Errorf("expected background job to finish before Wait returned, got %d", atomic.LoadInt32(&executed))
	}
}

func TestService_NoDispatchers(t *testing.T) {
	svc := NewService(nil, nil, time.Second)
	// Should not block or panic
	svc.DispatchAsync(NotificationRequest{Name: "NoOp", Email: "n@n.com", Message: "hello"})
	svc.Wait(100 * time.Millisecond)
}

func TestService_Telemetry_Success(t *testing.T) {
	var mu sync.Mutex
	receivedEvents := make([]TelemetryEvent, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type, got %s", r.Header.Get("Content-Type"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var event TelemetryEvent
		if err := json.Unmarshal(body, &event); err != nil {
			t.Errorf("failed to parse JSON telemetry event: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"recorded"}`))
	}))
	defer server.Close()

	emailMock := &mockEmailSender{}
	telegramMock := &mockTelegramSender{}

	svc := NewService(emailMock, telegramMock, 1*time.Second, server.URL)

	req := NotificationRequest{
		Name:    "Sensitive User",
		Email:   "sensitive@private.org",
		Message: "Super secret financial message",
		Source:  "laravel",
	}

	svc.DispatchAsync(req)
	svc.Wait(1 * time.Second)

	// Wait for background telemetry goroutines
	deadline := time.Now().Add(1 * time.Second)
	for {
		mu.Lock()
		count := len(receivedEvents)
		mu.Unlock()
		if count >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 telemetry events (email + telegram), got %d", len(receivedEvents))
	}

	channelEvents := make(map[string]TelemetryEvent)
	for _, ev := range receivedEvents {
		if ev.Source != "notifier" {
			t.Errorf("expected event source 'notifier', got %q", ev.Source)
		}
		if ev.Type != "notification_sent" {
			t.Errorf("expected event type 'notification_sent', got %q", ev.Type)
		}

		// Critical Privacy Rule: Strictly NO PII in metadata
		for k, v := range ev.Metadata {
			kLower := strings.ToLower(k)
			if kLower == "email" || kLower == "name" || kLower == "message" || kLower == "content" {
				t.Fatalf("CRITICAL PRIVACY VIOLATION: PII field %q found in metadata", k)
			}
			if strings.Contains(v, "sensitive@private.org") || strings.Contains(v, "Sensitive User") || strings.Contains(v, "secret") {
				t.Fatalf("CRITICAL PRIVACY VIOLATION: PII value leaked in metadata key %q: %q", k, v)
			}
		}

		ch := ev.Metadata["channel"]
		channelEvents[ch] = ev

		if ev.Metadata["backend"] != "laravel" {
			t.Errorf("expected backend 'laravel', got %q", ev.Metadata["backend"])
		}

		durStr := ev.Metadata["duration_ms"]
		durVal, err := strconv.Atoi(durStr)
		if err != nil || durVal < 0 {
			t.Errorf("invalid duration_ms in metadata: %q (err: %v)", durStr, err)
		}
	}

	if _, ok := channelEvents["email"]; !ok {
		t.Errorf("expected email telemetry event, but none was received")
	}
	if _, ok := channelEvents["telegram"]; !ok {
		t.Errorf("expected telegram telemetry event, but none was received")
	}
}

func TestService_Telemetry_Failure(t *testing.T) {
	var mu sync.Mutex
	receivedEvents := make([]TelemetryEvent, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var event TelemetryEvent
		_ = json.Unmarshal(body, &event)

		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	emailMock := &mockEmailSender{
		sendFunc: func(ctx context.Context, req NotificationRequest) error {
			return errors.New("resend 403 forbidden: domain unverified")
		},
	}
	telegramMock := &mockTelegramSender{
		sendFunc: func(ctx context.Context, req NotificationRequest) error {
			return errors.New("telegram 401 unauthorized: invalid bot token")
		},
	}

	svc := NewService(emailMock, telegramMock, 1*time.Second, server.URL)

	req := NotificationRequest{
		Name:    "Alice",
		Email:   "alice@example.com",
		Message: "Hello failure test",
		Source:  "spring",
	}

	svc.DispatchAsync(req)
	svc.Wait(1 * time.Second)

	deadline := time.Now().Add(1 * time.Second)
	for {
		mu.Lock()
		count := len(receivedEvents)
		mu.Unlock()
		if count >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 failure telemetry events, got %d", len(receivedEvents))
	}

	for _, ev := range receivedEvents {
		if ev.Source != "notifier" {
			t.Errorf("expected source 'notifier', got %q", ev.Source)
		}
		if ev.Type != "notification_failed" {
			t.Errorf("expected type 'notification_failed', got %q", ev.Type)
		}
		if ev.Metadata["backend"] != "spring" {
			t.Errorf("expected backend 'spring', got %q", ev.Metadata["backend"])
		}
		if ev.Metadata["error"] == "" {
			t.Errorf("expected error field in metadata for notification_failed event")
		}

		// Ensure no PII leaked
		if _, exists := ev.Metadata["email"]; exists {
			t.Errorf("PII email leaked in metadata")
		}
		if _, exists := ev.Metadata["name"]; exists {
			t.Errorf("PII name leaked in metadata")
		}
		if _, exists := ev.Metadata["message"]; exists {
			t.Errorf("PII message leaked in metadata")
		}
	}
}

func TestService_Telemetry_Disabled(t *testing.T) {
	var httpCalled int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	emailMock := &mockEmailSender{}

	// Pass empty string for monitorURL
	svc := NewService(emailMock, nil, 1*time.Second, "")

	if svc.MonitorURL() != "" {
		t.Errorf("expected empty MonitorURL, got %q", svc.MonitorURL())
	}

	svc.DispatchAsync(NotificationRequest{
		Name:    "Bob",
		Email:   "bob@example.com",
		Message: "Disabled test",
		Source:  "express",
	})
	svc.Wait(300 * time.Millisecond)

	if atomic.LoadInt32(&httpCalled) != 0 {
		t.Errorf("expected 0 HTTP telemetry calls when monitor is disabled, got %d", atomic.LoadInt32(&httpCalled))
	}
}

func TestService_Telemetry_MonitorOffline(t *testing.T) {
	emailMock := &mockEmailSender{}

	// Unreachable dead port
	offlineURL := "http://127.0.0.1:54321/api/events"
	svc := NewService(emailMock, nil, 1*time.Second, offlineURL)

	start := time.Now()
	svc.DispatchAsync(NotificationRequest{
		Name:    "OfflineTest",
		Email:   "offline@example.com",
		Message: "Testing offline monitor",
		Source:  "go",
	})

	// Must finish promptly and swallow connection refused without crashing
	svc.Wait(500 * time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("service took too long during offline monitor dispatch: %v", elapsed)
	}
}

func TestService_Telemetry_SlowMonitor_NonBlocking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the 100ms timeout
		time.Sleep(250 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	emailMock := &mockEmailSender{}
	svc := NewService(emailMock, nil, 1*time.Second, server.URL)

	start := time.Now()
	svc.DispatchAsync(NotificationRequest{
		Name:    "SlowMonitorTest",
		Email:   "slow@example.com",
		Message: "Testing non-blocking slow monitor",
		Source:  "express",
	})
	dispatchElapsed := time.Since(start)

	// DispatchAsync itself must return immediately
	if dispatchElapsed > 50*time.Millisecond {
		t.Errorf("DispatchAsync was blocked by telemetry, took %v", dispatchElapsed)
	}

	// Graceful wait handles bounded timeout
	svc.Wait(500 * time.Millisecond)
}

func TestService_Telemetry_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	emailMock := &mockEmailSender{}
	svc := NewService(emailMock, nil, 1*time.Second, server.URL)

	// Must not panic or return error when monitor returns 500
	svc.DispatchAsync(NotificationRequest{
		Name:    "ServerErrTest",
		Email:   "err@example.com",
		Message: "Testing server error resilience",
	})
	svc.Wait(300 * time.Millisecond)
}

func TestService_Telemetry_DefaultBackend_Unknown(t *testing.T) {
	var mu sync.Mutex
	var receivedEvent TelemetryEvent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		_ = json.Unmarshal(body, &receivedEvent)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	emailMock := &mockEmailSender{}
	svc := NewService(emailMock, nil, 1*time.Second, server.URL)

	// Source is omitted / empty
	svc.DispatchAsync(NotificationRequest{
		Name:    "NoSourceUser",
		Email:   "nosource@example.com",
		Message: "Testing default source fallback",
	})
	svc.Wait(500 * time.Millisecond)

	mu.Lock()
	backend := receivedEvent.Metadata["backend"]
	mu.Unlock()

	if backend != "unknown" {
		t.Errorf("expected backend 'unknown' when Source is empty, got %q", backend)
	}
}
