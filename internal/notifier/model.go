package notifier

import (
	"errors"
	"net/mail"
	"strings"
)

type NotificationRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
	Source  string `json:"source,omitempty"`
	Gotcha  string `json:"gotcha,omitempty"`
}

func (r *NotificationRequest) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Email = strings.TrimSpace(r.Email)
	r.Message = strings.TrimSpace(r.Message)
	r.Source = strings.TrimSpace(r.Source)

	if r.Name == "" {
		return errors.New("name is required")
	}
	if len(r.Name) > 200 {
		return errors.New("name exceeds maximum length of 200 characters")
	}
	if r.Email == "" {
		return errors.New("email is required")
	}
	if len(r.Email) > 254 {
		return errors.New("email exceeds maximum length of 254 characters")
	}
	if _, err := mail.ParseAddress(r.Email); err != nil {
		return errors.New("invalid email address")
	}
	if r.Message == "" {
		return errors.New("message is required")
	}
	if len(r.Message) > 10000 {
		return errors.New("message exceeds maximum length of 10000 characters")
	}
	if r.Source == "" {
		r.Source = "unknown"
	}
	return nil
}

func (r *NotificationRequest) IsHoneypot() bool {
	return strings.TrimSpace(r.Gotcha) != ""
}
