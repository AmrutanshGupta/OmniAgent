# ── Build Stage ──────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /app

# Download dependencies first (cached layer unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o orchestrator ./cmd/api

# ── Runtime Stage ─────────────────────────────────────────────────────────────
FROM alpine:3.19

# Install ca-certificates for HTTPS outbound calls (LLM APIs, OpenRouter, etc.)
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /app/orchestrator .

# Non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8080
CMD ["./orchestrator"]
