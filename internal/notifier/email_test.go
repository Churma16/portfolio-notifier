package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendSender_SendEmail_Success(t *testing.T) {
	var receivedPayload resendPayload
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": "re_12345"}`))
	}))
	defer server.Close()

	sender := NewResendSender(ResendConfig{
		APIKey:    "test_api_key",
		FromEmail: "test@churma.codes",
		ToEmail:   "target@example.com",
		BaseURL:   server.URL,
	})

	req := NotificationRequest{
		Name:    "John Doe",
		Email:   "john@example.com",
		Message: "Hello Resend!",
		Source:  "go",
	}

	err := sender.SendEmail(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuth != "Bearer test_api_key" {
		t.Errorf("expected Bearer test_api_key, got %q", receivedAuth)
	}
	if len(receivedPayload.To) == 0 || receivedPayload.To[0] != "target@example.com" {
		t.Errorf("expected to target@example.com, got %v", receivedPayload.To)
	}
	if receivedPayload.ReplyTo != "john@example.com" {
		t.Errorf("expected reply_to john@example.com, got %s", receivedPayload.ReplyTo)
	}
}

func TestResendSender_SendEmail_MissingKey(t *testing.T) {
	sender := NewResendSender(ResendConfig{
		APIKey: "",
	})

	err := sender.SendEmail(context.Background(), NotificationRequest{Name: "A", Email: "a@b.com", Message: "m"})
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestResendSender_SendEmail_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	sender := NewResendSender(ResendConfig{
		APIKey:  "test_api_key",
		BaseURL: server.URL,
	})

	err := sender.SendEmail(context.Background(), NotificationRequest{Name: "A", Email: "a@b.com", Message: "m"})
	if err == nil {
		t.Fatal("expected error for 400 status code, got nil")
	}
}
