# portfolio-notifier

Ultra-lightweight notification microservice for `churma.codes`.

This service centralizes incoming contact form submissions from all 4 portfolio backends (Laravel, Go, Express, Spring Boot) and concurrently dispatches notifications to:
1. **Email** (via Resend REST API)
2. **Telegram Bot** (via Telegram Bot REST API)

## Key Features

- **Zero External Dependencies**: Pure Go standard library (`net/http`, `context`, `encoding/json`).
- **Ultra-low Footprint**: Uses ~10 MB RAM, static binary ~10 MB, boots in < 5ms.
- **Asynchronous & Non-blocking**: HTTP caller receives `202 Accepted` immediately while delivery happens in background goroutines.
- **Resilient**: Bounded execution timeout (default 15s) and independent error handling for Email and Telegram.
- **Honeypot Protection**: Rejects bot spam quietly when the `gotcha` honeypot field is filled.

## API Endpoints

### 1. Health Check
```http
GET /health
```
**Response (200 OK):**
```json
{
  "service": "portfolio-notifier",
  "status": "healthy"
}
```

### 2. Dispatch Notification
```http
POST /api/notify
Content-Type: application/json
```
**Payload:**
```json
{
  "name": "John Doe",
  "email": "john@example.com",
  "message": "Halo, tertarik kolaborasi project!",
  "source": "laravel",
  "gotcha": ""
}
```
**Response (202 Accepted):**
```json
{
  "message": "Notification queued for delivery",
  "status": "accepted"
}
```

## Running Tests

Run tests hermetically using Docker:
```bash
docker run --rm -v $(pwd):/app -w /app golang:alpine go test -v -race ./...
```
