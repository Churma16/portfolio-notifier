package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegramBotSender_SendTelegram_Success(t *testing.T) {
	var receivedPayload telegramPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true, "result": {}}`))
	}))
	defer server.Close()

	sender := NewTelegramBotSender(TelegramConfig{
		BotToken: "test_bot_token",
		ChatID:   "123456789",
		BaseURL:  server.URL,
	})

	req := NotificationRequest{
		Name:    "Jane Doe",
		Email:   "jane@example.com",
		Message: "Halo <script>alert(1)</script> & test",
		Source:  "laravel",
	}

	err := sender.SendTelegram(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload.ChatID != "123456789" {
		t.Errorf("expected chat_id 123456789, got %q", receivedPayload.ChatID)
	}
	if receivedPayload.ParseMode != "HTML" {
		t.Errorf("expected parse_mode HTML, got %q", receivedPayload.ParseMode)
	}
	// Verify HTML is escaped
	if strings.Contains(receivedPayload.Text, "<script>") {
		t.Errorf("expected HTML to be escaped, found raw <script>")
	}
	if !strings.Contains(receivedPayload.Text, "&lt;script&gt;") {
		t.Errorf("expected escaped &lt;script&gt; in message text, got: %s", receivedPayload.Text)
	}
}

func TestTelegramBotSender_SendTelegram_MissingCredentials(t *testing.T) {
	sender := NewTelegramBotSender(TelegramConfig{
		BotToken: "",
		ChatID:   "",
	})

	err := sender.SendTelegram(context.Background(), NotificationRequest{Name: "A", Email: "a@b.com", Message: "m"})
	if err == nil {
		t.Fatal("expected error for missing credentials, got nil")
	}
}

func TestTelegramBotSender_SendTelegram_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok": false, "description": "Unauthorized"}`))
	}))
	defer server.Close()

	sender := NewTelegramBotSender(TelegramConfig{
		BotToken: "test_token",
		ChatID:   "123",
		BaseURL:  server.URL,
	})

	err := sender.SendTelegram(context.Background(), NotificationRequest{Name: "A", Email: "a@b.com", Message: "m"})
	if err == nil {
		t.Fatal("expected error for 401 status code, got nil")
	}
}
