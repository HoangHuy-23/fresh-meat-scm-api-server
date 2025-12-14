# Multi-stage Dockerfile for fresh-meat-scm-api-server
# Build stage
FROM golang:1.23 AS builder

WORKDIR /app

# Enable Go modules and configure build
ENV CGO_ENABLED=0 \
    GO111MODULE=on \
    GOTOOLCHAIN=auto

# Pre-cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source
COPY . .

# Build the API binary
RUN go build -o /bin/api ./cmd/api

# Runtime stage
FROM alpine:3.20

# Install CA certificates for TLS (MongoDB, S3, etc.)
RUN apk add --no-cache ca-certificates tzdata && \
    update-ca-certificates

# Create runtime user
RUN adduser -D -H -u 10001 appuser

WORKDIR /srv/app

# Copy binary from builder
COPY --from=builder /bin/api /usr/local/bin/api

# Copy non-secret runtime configs (adjust as needed)
COPY connection-profile.yaml ./
COPY config/config.yaml ./config/

# Prepare writable runtime directories for SDK wallets/keystores
RUN mkdir -p /srv/app/wallet /srv/app/keystore && \
    chown -R appuser:appuser /srv/app

# If your app needs static assets, uncomment and copy them similarly
# COPY internal/... ./internal/ ...

# Set environment variables (override at runtime)
ENV GIN_MODE=release \
    PORT=8080

# Expose API port
EXPOSE 8080

# Run as non-root
USER appuser

# Entrypoint
ENTRYPOINT ["/usr/local/bin/api"]
# Optionally pass flags via CMD (none by default)
CMD []
