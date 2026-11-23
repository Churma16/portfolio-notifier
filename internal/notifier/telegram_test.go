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

func TestTelegramBotSender_SendTelegram_TruncateLongMessage(t *testing.T) {
	var receivedPayload telegramPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	sender := NewTelegramBotSender(TelegramConfig{
		BotToken: "test_bot_token",
		ChatID:   "123",
		BaseURL:  server.URL,
	})

	longMsg := strings.Repeat("A", 4000)
	req := NotificationRequest{
		Name:    "Jane",
		Email:   "jane@example.com",
		Message: longMsg,
	}

	err := sender.SendTelegram(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(receivedPayload.Text, "... (pesan dipotong)") {
		t.Error("expected truncated message indicator in text")
	}
}

func TestTelegramBotSender_SendTelegram_HtmlExpansionTruncation(t *testing.T) {
	var receivedPayload telegramPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	sender := NewTelegramBotSender(TelegramConfig{
		BotToken: "test_bot_token",
		ChatID:   "123",
		BaseURL:  server.URL,
	})

	// 1500 "<" characters expands to 6000 chars when escaped as "&lt;"
	denseHtmlMsg := strings.Repeat("<", 1500)
	req := NotificationRequest{
		Name:    "Coder",
		Email:   "coder@example.com",
		Message: denseHtmlMsg,
	}

	err := sender.SendTelegram(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Telegram sendMessage strict limit is 4096 UTF-8 characters
	if len([]rune(receivedPayload.Text)) > 4000 {
		t.Errorf("payload text length %d exceeds safe limit 4000", len([]rune(receivedPayload.Text)))
	}
	if !strings.Contains(receivedPayload.Text, "... (pesan dipotong)") {
		t.Error("expected truncated indicator for HTML-expanded message")
	}
	// Verify no broken trailing entity like &l or &am
	if strings.HasSuffix(strings.TrimSuffix(receivedPayload.Text, "\n\n... (pesan dipotong)"), "&") {
		t.Error("message should not end with bare '&' entity start")
	}
}

func TestTelegramBotSender_SendTelegram_ExtremeEscapedMetadata(t *testing.T) {
	var receivedPayload telegramPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	sender := NewTelegramBotSender(TelegramConfig{
		BotToken: "test_bot_token",
		ChatID:   "123",
		BaseURL:  server.URL,
	})

	// 200 '&' characters expand to 1000 chars as '&amp;'
	extremeName := strings.Repeat("&", 200)
	req := NotificationRequest{
		Name:    extremeName,
		Email:   "extreme@example.com",
		Message: strings.Repeat("A", 4000),
		Source:  "laravel",
	}

	err := sender.SendTelegram(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must strictly be <= 4000 runes
	totalRunes := len([]rune(receivedPayload.Text))
	if totalRunes > 4000 {
		t.Errorf("total runes %d exceeds 4000 max budget", totalRunes)
	}
	if !strings.Contains(receivedPayload.Text, "... (pesan dipotong)") {
		t.Error("expected message to be truncated")
	}
}

func TestTelegramBotSender_SendTelegram_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok": false, "description": "Unauthorized token"}`))
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
	if !strings.Contains(err.Error(), "Unauthorized token") {
		t.Errorf("expected error message to contain response snippet, got: %v", err)
	}
}
