# Build stage
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

# Build the application
RUN go build -o hypasis-sync ./cmd/hypasis-sync

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/hypasis-sync /app/

# Create data directory
RUN mkdir -p /data/hypasis

# Expose ports
EXPOSE 8080 9090 30303

# Set default data directory
ENV DATA_DIR=/data/hypasis

# Run the application
ENTRYPOINT ["/app/hypasis-sync"]
CMD ["--data-dir=/data/hypasis"]
