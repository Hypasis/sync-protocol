# Multi-stage build for Hypasis Sync Protocol - Cloud Production
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev linux-headers

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download || true

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags '-extldflags "-static" -s -w' \
    -o hypasis-sync ./cmd/hypasis-sync

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user for security
RUN addgroup -g 1000 hypasis && \
    adduser -D -u 1000 -G hypasis hypasis

# Create data directory
RUN mkdir -p /data/hypasis /etc/hypasis/tls && \
    chown -R hypasis:hypasis /data/hypasis /etc/hypasis

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/hypasis-sync .

# Copy configuration files
COPY config.example.yaml ./config.example.yaml
COPY config.cloud.yaml ./config.cloud.yaml

# Switch to non-root user
USER hypasis

# Expose ports
# 8080: REST API
# 8545: RPC for Bor
# 9090: Prometheus metrics
# 30303: DevP2P (TCP + UDP)
EXPOSE 8080 8545 9090 30303 30303/udp

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set environment variables
ENV DATA_DIR=/data/hypasis

# Run the application
ENTRYPOINT ["/app/hypasis-sync"]
CMD ["--config", "/app/config.cloud.yaml"]
