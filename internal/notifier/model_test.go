package notifier

import (
	"testing"
)

func TestNotificationRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     NotificationRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: NotificationRequest{
				Name:    "Alice",
				Email:   "alice@example.com",
				Message: "Hello there!",
				Source:  "laravel",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			req: NotificationRequest{
				Name:    "",
				Email:   "alice@example.com",
				Message: "Hello",
			},
			wantErr: true,
		},
		{
			name: "missing email",
			req: NotificationRequest{
				Name:    "Alice",
				Email:   "",
				Message: "Hello",
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			req: NotificationRequest{
				Name:    "Alice",
				Email:   "invalid-email",
				Message: "Hello",
			},
			wantErr: true,
		},
		{
			name: "missing message",
			req: NotificationRequest{
				Name:    "Alice",
				Email:   "alice@example.com",
				Message: "   ",
			},
			wantErr: true,
		},
		{
			name: "default source to unknown if empty",
			req: NotificationRequest{
				Name:    "Bob",
				Email:   "bob@example.com",
				Message: "Test",
				Source:  "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.req.Source == "" {
				t.Errorf("expected Source to default to unknown, got %q", tt.req.Source)
			}
		})
	}
}

func TestNotificationRequest_IsHoneypot(t *testing.T) {
	reqEmpty := NotificationRequest{Gotcha: ""}
	if reqEmpty.IsHoneypot() {
		t.Errorf("expected false for empty gotcha")
	}

	reqSpam := NotificationRequest{Gotcha: "spam-bot"}
	if !reqSpam.IsHoneypot() {
		t.Errorf("expected true for filled gotcha")
	}
}
