package notifier

import (
	"context"
	"errors"
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
