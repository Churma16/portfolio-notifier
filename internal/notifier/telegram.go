package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"time"
)

type TelegramSender interface {
	SendTelegram(ctx context.Context, req NotificationRequest) error
}

type TelegramConfig struct {
	BotToken string
	ChatID   string
	BaseURL  string // defaults to https://api.telegram.org/bot<TOKEN>/sendMessage
	Client   *http.Client
}

type TelegramBotSender struct {
	cfg TelegramConfig
}

func NewTelegramBotSender(cfg TelegramConfig) *TelegramBotSender {
	if cfg.BaseURL == "" && cfg.BotToken != "" {
		cfg.BaseURL = fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.BotToken)
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	return &TelegramBotSender{cfg: cfg}
}

type telegramPayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func (s *TelegramBotSender) SendTelegram(ctx context.Context, req NotificationRequest) error {
	if s.cfg.BotToken == "" || s.cfg.ChatID == "" {
		return fmt.Errorf("telegram bot token or chat id is not configured")
	}

	targetURL := s.cfg.BaseURL
	if targetURL == "" {
		targetURL = fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.cfg.BotToken)
	}

	msg := fmt.Sprintf(
		"<b>📩 Pesan Baru (churma.codes)</b>\n"+
			"<b>⚙️ Backend:</b> <code>%s</code>\n"+
			"<b>👤 Pengirim:</b> %s\n"+
			"<b>📧 Email:</b> %s\n\n"+
			"<b>💬 Pesan:</b>\n%s",
		html.EscapeString(req.Source),
		html.EscapeString(req.Name),
		html.EscapeString(req.Email),
		html.EscapeString(req.Message),
	)

	payload := telegramPayload{
		ChatID:    s.cfg.ChatID,
		Text:      msg,
		ParseMode: "HTML",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.cfg.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("telegram request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram returned non-2xx status code: %d", resp.StatusCode)
	}

	return nil
}
