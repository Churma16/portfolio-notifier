package notifier

import (
	"context"
	"log"
	"sync"
	"time"
)

type Service struct {
	emailSender    EmailSender
	telegramSender TelegramSender
	timeout        time.Duration
	inFlight       sync.WaitGroup
}

func NewService(emailSender EmailSender, telegramSender TelegramSender, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Service{
		emailSender:    emailSender,
		telegramSender: telegramSender,
		timeout:        timeout,
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
				} else {
					log.Printf("[SUCCESS] [Email] Email delivered for %s (latency: %v)", req.Email, time.Since(start))
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
				} else {
					log.Printf("[SUCCESS] [Telegram] Telegram notification delivered (latency: %v)", time.Since(start))
				}
			}()
		}

		wg.Wait()
	}()
}

// Wait blocks until all in-flight background notification dispatches complete or timeout expires.
func (s *Service) Wait(timeout time.Duration) {
	c := make(chan struct{})
	go func() {
		s.inFlight.Wait()
		close(c)
	}()

	select {
	case <-c:
	case <-time.After(timeout):
		log.Println("[WARN] Timed out waiting for in-flight notifications during shutdown")
	}
}
