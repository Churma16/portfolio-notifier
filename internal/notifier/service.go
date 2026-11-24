package notifier

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Service struct {
	emailSender     EmailSender
	telegramSender  TelegramSender
	timeout         time.Duration
	monitorURL      string
	telemetryClient *http.Client
	inFlight        sync.WaitGroup
	telemetryWg     sync.WaitGroup
}

func NewService(emailSender EmailSender, telegramSender TelegramSender, timeout time.Duration, monitorURL ...string) *Service {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	var url string
	if len(monitorURL) > 0 {
		url = strings.TrimSpace(monitorURL[0])
	}
	return &Service{
		emailSender:     emailSender,
		telegramSender:  telegramSender,
		timeout:         timeout,
		monitorURL:      url,
		telemetryClient: defaultTelemetryClient,
	}
}

// SetMonitorURL updates or configures the monitor URL.
func (s *Service) SetMonitorURL(url string) {
	s.monitorURL = strings.TrimSpace(url)
}

// MonitorURL returns the configured monitor URL.
func (s *Service) MonitorURL() string {
	return s.monitorURL
}

// SetTelemetryClient allows overriding the HTTP client (useful for unit testing).
func (s *Service) SetTelemetryClient(client *http.Client) {
	s.telemetryClient = client
}

// WaitTelemetry blocks until all in-flight telemetry dispatches finish or timeout expires.
func (s *Service) WaitTelemetry(timeout time.Duration) {
	c := make(chan struct{})
	go func() {
		s.telemetryWg.Wait()
		close(c)
	}()

	select {
	case <-c:
	case <-time.After(timeout):
	}
}

func (s *Service) DispatchAsync(req NotificationRequest) {
	if s.emailSender == nil && s.telegramSender == nil {
		log.Println("[WARN] No notification dispatchers enabled; notification dropped")
		return
	}

	s.inFlight.Add(1)
	go func() {
		defer s.inFlight.Done()

		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()

		var wg sync.WaitGroup

		if s.emailSender != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				start := time.Now()
				if err := s.emailSender.SendEmail(ctx, req); err != nil {
					log.Printf("[ERROR] [Email] Failed to deliver email for %s: %v (latency: %v)", req.Email, err, time.Since(start))
					s.dispatchTelemetry("email", "notification_failed", time.Since(start), req.Source, err)
				} else {
					log.Printf("[SUCCESS] [Email] Email delivered for %s (latency: %v)", req.Email, time.Since(start))
					s.dispatchTelemetry("email", "notification_sent", time.Since(start), req.Source, nil)
				}
			}()
		}

		if s.telegramSender != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				start := time.Now()
				if err := s.telegramSender.SendTelegram(ctx, req); err != nil {
					log.Printf("[ERROR] [Telegram] Failed to deliver telegram notification: %v (latency: %v)", err, time.Since(start))
					s.dispatchTelemetry("telegram", "notification_failed", time.Since(start), req.Source, err)
				} else {
					log.Printf("[SUCCESS] [Telegram] Telegram notification delivered (latency: %v)", time.Since(start))
					s.dispatchTelemetry("telegram", "notification_sent", time.Since(start), req.Source, nil)
				}
			}()
		}

		wg.Wait()
	}()
}

// Wait blocks until all in-flight background notification dispatches complete or timeout expires.
func (s *Service) Wait(timeout time.Duration) {
	start := time.Now()
	c := make(chan struct{})
	go func() {
		s.inFlight.Wait()
		close(c)
	}()

	select {
	case <-c:
	case <-time.After(timeout):
		log.Println("[WARN] Timed out waiting for in-flight notifications during shutdown")
		return
	}

	remaining := timeout - time.Since(start)
	if remaining > 0 && s.monitorURL != "" {
		s.WaitTelemetry(min(remaining, 200*time.Millisecond))
	}
}
