# Build Stage
FROM golang:alpine AS builder

WORKDIR /app

# Copy module manifests first for caching
COPY go.mod ./
RUN go mod download || true

# Copy source code and build statically
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/notifier .

# Minimal Production Image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

COPY --from=builder /app/notifier /app/notifier

EXPOSE 8080
USER nobody:nobody

ENTRYPOINT ["/app/notifier"]
