# syntax=docker/dockerfile:1

# ============================================================================
# Build Stage
# ============================================================================
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev}" \
    -o /omniproxy \
    ./cmd/omniproxy

# ============================================================================
# Runtime Stage
# ============================================================================
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -S omniproxy && adduser -S omniproxy -G omniproxy

# Create directories for data and config
RUN mkdir -p /data /config && chown -R omniproxy:omniproxy /data /config

# Copy binary from builder
COPY --from=builder /omniproxy /usr/local/bin/omniproxy

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Set working directory
WORKDIR /data

# Switch to non-root user
USER omniproxy

# Expose ports
# 8080: Proxy port
# 9090: Metrics/health port
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9090/healthz || exit 1

# Default environment variables
ENV OMNIPROXY_HOST=0.0.0.0 \
    OMNIPROXY_PORT=8080 \
    OMNIPROXY_METRICS_PORT=9090

# Default command
ENTRYPOINT ["omniproxy"]
CMD ["serve", "--host", "0.0.0.0", "--port", "8080", "--metrics-port", "9090"]
