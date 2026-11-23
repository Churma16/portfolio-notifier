package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Churma16/portfolio-notifier/internal/notifier"
)

type Config struct {
	Port              string
	ResendAPIKey      string
	ResendFromEmail   string
	ResendToEmail     string
	TelegramBotToken  string
	TelegramChatID    string
	TimeoutSeconds    int
}

func loadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fromEmail := os.Getenv("RESEND_FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "contact@churma.codes"
	}

	toEmail := os.Getenv("RESEND_TO_EMAIL")
	if toEmail == "" {
		toEmail = "fathanmf16@gmail.com"
	}

	timeoutSec := 15
	if rawTimeout := os.Getenv("TIMEOUT_SECONDS"); rawTimeout != "" {
		if val, err := strconv.Atoi(rawTimeout); err == nil && val > 0 {
			timeoutSec = val
		}
	}

	return Config{
		Port:             port,
		ResendAPIKey:     os.Getenv("RESEND_API_KEY"),
		ResendFromEmail:  fromEmail,
		ResendToEmail:    toEmail,
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		TimeoutSeconds:   timeoutSec,
	}
}

func SetupRouter(svc *notifier.Service) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"service": "portfolio-notifier",
		})
	})

	notifyHandler := func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

		var req notifier.NotificationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid or oversized JSON payload"})
			return
		}

		if req.IsHoneypot() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":  "ok",
				"message": "Notification accepted",
			})
			return
		}

		if err := req.Validate(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		svc.DispatchAsync(req)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "accepted",
			"message": "Notification queued for delivery",
		})
	}

	mux.HandleFunc("POST /api/notify", notifyHandler)
	mux.HandleFunc("POST /notify", notifyHandler)

	return mux
}

func main() {
	cfg := loadConfig()

	log.Printf("[INFO] Initializing portfolio-notifier service...")

	var emailSender notifier.EmailSender
	if cfg.ResendAPIKey != "" {
		emailSender = notifier.NewResendSender(notifier.ResendConfig{
			APIKey:    cfg.ResendAPIKey,
			FromEmail: cfg.ResendFromEmail,
			ToEmail:   cfg.ResendToEmail,
		})
		log.Printf("[INFO] Resend email sender enabled (to: %s, from: %s)", cfg.ResendToEmail, cfg.ResendFromEmail)
	} else {
		log.Printf("[WARN] RESEND_API_KEY is not set. Email delivery is disabled.")
	}

	var telegramSender notifier.TelegramSender
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		telegramSender = notifier.NewTelegramBotSender(notifier.TelegramConfig{
			BotToken: cfg.TelegramBotToken,
			ChatID:   cfg.TelegramChatID,
		})
		log.Printf("[INFO] Telegram notification sender enabled (chat_id: %s)", cfg.TelegramChatID)
	} else {
		log.Printf("[WARN] TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID is not set. Telegram delivery is disabled.")
	}

	svc := notifier.NewService(emailSender, telegramSender, time.Duration(cfg.TimeoutSeconds)*time.Second)
	router := SetupRouter(svc)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("[INFO] Server listening on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[FATAL] Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[INFO] Shutting down server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Server forced to shutdown: %v", err)
	}

	log.Println("[INFO] Waiting for in-flight notifications to complete...")
	shutdownWait := min(time.Duration(cfg.TimeoutSeconds)*time.Second, 8*time.Second)
	svc.Wait(shutdownWait)
	log.Println("[INFO] Server stopped gracefully.")
}
