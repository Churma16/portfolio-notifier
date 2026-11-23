package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

type EmailSender interface {
	SendEmail(ctx context.Context, req NotificationRequest) error
}

type ResendConfig struct {
	APIKey    string
	FromEmail string
	ToEmail   string
	BaseURL   string // defaults to https://api.resend.com/emails
	Client    *http.Client
}

type ResendSender struct {
	cfg ResendConfig
}

func NewResendSender(cfg ResendConfig) *ResendSender {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.resend.com/emails"
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	return &ResendSender{cfg: cfg}
}

type resendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	ReplyTo string   `json:"reply_to,omitempty"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

func (s *ResendSender) SendEmail(ctx context.Context, req NotificationRequest) error {
	if s.cfg.APIKey == "" {
		return fmt.Errorf("resend api key is not configured")
	}

	subject := fmt.Sprintf("[churma.codes] Pesan Baru dari %s", req.Name)
	bodyHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; line-height: 1.6; color: #1e293b; padding: 20px;">
  <div style="max-width: 600px; margin: 0 auto; border: 1px solid #e2e8f0; border-radius: 8px; padding: 24px; background: #ffffff;">
    <h2 style="color: #0f172a; margin-top: 0; border-bottom: 2px solid #f1f5f9; padding-bottom: 12px;">Pesan Baru (churma.codes)</h2>
    <p style="margin: 8px 0;"><strong>⚙️ Sumber Backend:</strong> <code>%s</code></p>
    <p style="margin: 8px 0;"><strong>👤 Nama Pengirim:</strong> %s</p>
    <p style="margin: 8px 0;"><strong>📧 Email Pengirim:</strong> <a href="mailto:%s" style="color: #2563eb;">%s</a></p>
    <div style="margin-top: 20px;">
      <strong>💬 Isi Pesan:</strong>
      <div style="background: #f8fafc; border-left: 4px solid #3b82f6; padding: 14px; border-radius: 4px; margin-top: 8px; white-space: pre-wrap; font-family: inherit;">%s</div>
    </div>
  </div>
</body>
</html>`,
		html.EscapeString(req.Source),
		html.EscapeString(req.Name),
		html.EscapeString(req.Email),
		html.EscapeString(req.Email),
		strings.ReplaceAll(html.EscapeString(req.Message), "\n", "<br>"),
	)

	payload := resendPayload{
		From:    fmt.Sprintf("Portfolio Contact <%s>", s.cfg.FromEmail),
		To:      []string{s.cfg.ToEmail},
		ReplyTo: req.Email,
		Subject: subject,
		HTML:    bodyHTML,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal resend payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BaseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create resend request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.cfg.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("resend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend returned non-2xx status code: %d", resp.StatusCode)
	}

	return nil
}
