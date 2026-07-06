# ============================================================
# Build stage
# ============================================================
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build API server
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /app/bin/fury-sms-gateway ./cmd/api

# Build bootstrap command
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /app/bin/fury-bootstrap ./cmd/bootstrap

# ============================================================
# Runtime stage
# ============================================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binaries
COPY --from=builder /app/bin/ /app/
COPY --from=builder /app/migrations/ /app/migrations/

# Create non-root user
RUN adduser -D -h /app fury && chown -R fury:fury /app
USER fury

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/fury-sms-gateway"]
