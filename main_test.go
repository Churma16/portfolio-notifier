package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Churma16/portfolio-notifier/internal/notifier"
)

func TestSetupRouter_HealthCheck(t *testing.T) {
	svc := notifier.NewService(nil, nil, time.Second)
	router := SetupRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %q", res["status"])
	}
}

func TestSetupRouter_Notify_ValidationFailure(t *testing.T) {
	svc := notifier.NewService(nil, nil, time.Second)
	router := SetupRouter(svc)

	invalidPayload := map[string]string{
		"name": "John",
		// missing email and message
	}
	body, _ := json.Marshal(invalidPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/notify", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestSetupRouter_Notify_Honeypot(t *testing.T) {
	svc := notifier.NewService(nil, nil, time.Second)
	router := SetupRouter(svc)

	botPayload := map[string]string{
		"name":    "SpamBot",
		"email":   "bot@spam.com",
		"message": "Buy crypto",
		"gotcha":  "i-am-a-bot",
	}
	body, _ := json.Marshal(botPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/notify", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for honeypot, got %d", rec.Code)
	}

	var res map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", res["status"])
	}
}

func TestSetupRouter_Notify_Success(t *testing.T) {
	svc := notifier.NewService(nil, nil, time.Second)
	router := SetupRouter(svc)

	payload := map[string]string{
		"name":    "Jane Doe",
		"email":   "jane@example.com",
		"message": "Let's collaborate!",
		"source":  "spring",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/notify", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rec.Code)
	}

	var res map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["status"] != "accepted" {
		t.Errorf("expected status 'accepted', got %q", res["status"])
	}
}
