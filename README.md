# OmniProxy

[![Build Status][build-status-svg]][build-status-url]
[![Lint Status][lint-status-svg]][lint-status-url]
[![Go Report Card][goreport-svg]][goreport-url]
[![Docs][docs-godoc-svg]][docs-godoc-url]
[![Visualization][viz-svg]][viz-url]
[![License][license-svg]][license-url]

Universal HTTP/HTTPS proxy with MITM support for traffic capture and analysis.

## Features

- **Forward Proxy** - HTTP proxy for routing traffic
- **MITM Proxy** - HTTPS interception with automatic certificate generation
- **Reverse Proxy** - Server-side proxy with Let's Encrypt/ACME support
- **Traffic Capture** - Export to NDJSON, HAR, or IR formats
- **Database Storage** - SQLite (team mode) or PostgreSQL (production mode)
- **Request Filtering** - Include/exclude by host, path, or method
- **Binary Detection** - Automatically skip binary content (images, videos, etc.)
- **Proxy Chaining** - Forward through upstream proxy
- **Observability** - Prometheus metrics and health endpoints
- **System Proxy Configuration** - Automatic setup for macOS, Windows, and Linux
- **Config File Support** - YAML configuration files
- **Pure Go CA** - No OpenSSL dependency, uses Go's crypto libraries
- **Docker Support** - Multi-stage Dockerfile with health checks

## Deployment Modes

OmniProxy supports three deployment modes with the same binary:

| Mode | Storage | Use Case | Command |
|------|---------|----------|---------|
| **Laptop** | File (NDJSON) | Individual developer | `omniproxy serve --output traffic.ndjson` |
| **Team** | SQLite | Small team, shared server | `omniproxy serve --db sqlite://traffic.db` |
| **Production** | PostgreSQL | Enterprise, multi-tenant | `omniproxy serve --db postgres://...` |

## Installation

### From Source

```bash
go install github.com/grokify/omniproxy/cmd/omniproxy@latest
```

### Docker

```bash
# Build
docker build -t omniproxy .

# Run
docker run -p 8080:8080 -p 9090:9090 omniproxy serve --host 0.0.0.0

# Or use docker-compose
docker-compose up
```

## Quick Start

### 1. Generate and Install CA Certificate

```bash
# Generate CA (first time only)
omniproxy ca generate

# Install CA into system trust store (may require sudo)
sudo omniproxy ca install
```

### 2. Start the Proxy

```bash
# Laptop mode - file output
omniproxy serve --output traffic.ndjson

# Team mode - SQLite database
omniproxy serve --db sqlite://traffic.db --metrics-port 9090

# Production mode - PostgreSQL
omniproxy serve --db postgres://user:pass@localhost/omniproxy --metrics-port 9090
```

### 3. Configure System to Use Proxy

```bash
# Automatically configure system proxy
omniproxy system set --port 8080

# Check current proxy status
omniproxy system status

# Disable system proxy
omniproxy system unset
```

## Usage

### Serve Command

Start the proxy server:

```bash
omniproxy serve [flags]

Basic Flags:
  -p, --port int           Port to listen on (default 8080)
      --host string        Host to bind to (default "127.0.0.1")
  -v, --verbose            Enable verbose logging
      --mitm               Enable HTTPS MITM (default true)

Output Flags:
  -o, --output string      Output file for captured traffic
  -f, --format string      Output format: ndjson, json, har, ir (default "ndjson")
      --filter-header strings  Additional headers to filter
      --skip-binary        Skip capturing binary content (default true)

Database Flags:
      --db string          Database URL (sqlite://path or postgres://...)

Observability Flags:
      --metrics-port int   Port for metrics/health endpoints (0 = disabled)
      --metrics            Enable Prometheus metrics

Async Flags:
      --async-queue int    Async traffic queue size (default 10000)
      --async-batch int    Async batch size (default 100)
      --async-workers int  Number of async workers (default 2)

Filtering Flags:
      --include-host strings   Only capture requests to these hosts (wildcards supported)
      --exclude-host strings   Exclude requests to these hosts
      --include-path strings   Only capture requests matching these paths
      --exclude-path strings   Exclude requests matching these paths
      --include-method strings Only capture these HTTP methods
      --exclude-method strings Exclude these HTTP methods

Proxy Flags:
      --skip-host strings  Hosts to skip MITM for (cert pinning)
      --upstream string    Upstream proxy URL (e.g., http://proxy:8080)
```

### Database URLs

OmniProxy supports the following database URL formats:

```bash
# SQLite
sqlite://traffic.db              # Relative path
sqlite:///var/lib/omniproxy/data.db  # Absolute path
sqlite::memory:                  # In-memory (testing)
sqlite://data.db?cache=shared    # With options

# PostgreSQL
postgres://localhost/omniproxy
postgres://user:pass@localhost:5432/omniproxy
postgres://user:pass@localhost/omniproxy?sslmode=disable
postgresql://...                 # Alternative scheme
```

### Examples

```bash
# Capture only API traffic
omniproxy serve --include-host "api.example.com" --include-host "*.api.example.org"

# Exclude static assets
omniproxy serve --exclude-path "*.js" --exclude-path "*.css" --exclude-path "*.png"

# Capture only GET and POST requests
omniproxy serve --include-method GET --include-method POST

# Export to HAR format for browser DevTools
omniproxy serve --output traffic.har --format har

# Chain through corporate proxy
omniproxy serve --upstream http://corporate-proxy:8080

# Team mode with metrics
omniproxy serve --db sqlite://traffic.db --metrics-port 9090

# Production mode with all options
omniproxy serve \
  --db postgres://user:pass@localhost/omniproxy \
  --metrics-port 9090 \
  --async-queue 50000 \
  --async-batch 500 \
  --async-workers 4

# Use with config file
omniproxy serve --config ~/.omniproxy/config.yaml
```

### CA Commands

Manage CA certificates:

```bash
# Generate new CA
omniproxy ca generate [--cert path] [--key path] [--org name] [--cn name]

# Install CA to system trust store
omniproxy ca install [--cert path]

# Remove CA from system trust store
omniproxy ca uninstall

# Show CA information
omniproxy ca info
```

### System Commands

Manage system proxy configuration:

```bash
# Enable system proxy
omniproxy system set [--host host] [--port port]

# Disable system proxy
omniproxy system unset

# Show current status
omniproxy system status
```

### Reverse Command

Start a reverse proxy with automatic TLS via Let's Encrypt:

```bash
omniproxy reverse [flags]

Backend Flags:
  -b, --backend strings    Backend mapping: host=target (required)

ACME Flags:
      --acme-email string  Email for Let's Encrypt registration
      --acme-cache string  Directory to cache certificates (default "~/.omniproxy/acme")
      --acme-staging       Use Let's Encrypt staging environment (for testing)

Server Flags:
      --http-port int      HTTP port (default 80)
      --https-port int     HTTPS port (default 443)
      --redirect-http      Redirect HTTP to HTTPS (default true)
  -v, --verbose            Enable verbose logging

Output Flags:
  -o, --output string      Output file for captured traffic
  -f, --format string      Output format: ndjson, json, har, ir (default "ndjson")
```

#### Reverse Proxy Examples

```bash
# Basic reverse proxy with Let's Encrypt
sudo omniproxy reverse \
  --backend "api.example.com=http://localhost:3000" \
  --acme-email admin@example.com

# Multiple backends
sudo omniproxy reverse \
  --backend "api.example.com=http://localhost:3000" \
  --backend "web.example.com=http://localhost:8080" \
  --acme-email admin@example.com

# Use staging environment for testing
sudo omniproxy reverse \
  --backend "api.example.com=http://localhost:3000" \
  --acme-staging

# Custom ports (for local testing)
omniproxy reverse \
  --http-port 8080 \
  --https-port 8443 \
  --backend "localhost:8443=http://localhost:3000"

# With traffic capture
sudo omniproxy reverse \
  --backend "api.example.com=http://localhost:3000" \
  --output traffic.ndjson
```

**Note:** Running on ports 80 and 443 typically requires root/sudo.

### Config Commands

Manage configuration files:

```bash
# Create default config file
omniproxy config init

# Show example configuration
omniproxy config show
```

## Observability

When running with `--metrics-port`, OmniProxy exposes:

| Endpoint | Description |
|----------|-------------|
| `/metrics` | Prometheus metrics |
| `/healthz` | Liveness probe (always returns 200 when running) |
| `/readyz` | Readiness probe (returns 200 when ready to accept traffic) |

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `omniproxy_requests_total` | Counter | Total HTTP requests processed |
| `omniproxy_request_duration_seconds` | Histogram | Request duration in seconds |
| `omniproxy_active_requests` | Gauge | Currently active requests |
| `omniproxy_certs_generated_total` | Counter | TLS certificates generated |
| `omniproxy_cert_cache_hits_total` | Counter | Certificate cache hits |
| `omniproxy_cert_cache_misses_total` | Counter | Certificate cache misses |
| `omniproxy_traffic_stored_total` | Counter | Traffic records stored |
| `omniproxy_traffic_store_errors_total` | Counter | Traffic store errors |
| `omniproxy_traffic_queue_depth` | Gauge | Async queue depth |

## Configuration File

OmniProxy supports YAML configuration files:

```yaml
server:
  host: 127.0.0.1
  port: 8080
  verbose: false

mitm:
  enabled: true
  skipHosts:
    - "*.pinned-app.com"

capture:
  output: traffic.ndjson
  format: ndjson
  includeHeaders: true
  includeBody: true
  maxBodySize: 1048576
  filterHeaders:
    - authorization
    - cookie
    - set-cookie

filter:
  includeHosts:
    - "api.example.com"
    - "*.example.org"
  excludePaths:
    - "*.js"
    - "*.css"
    - "*.png"

upstream: ""
```

## Output Formats

### NDJSON (default)

Newline-delimited JSON, one record per line:

```json
{"request":{"method":"GET","url":"https://api.example.com/users","host":"api.example.com","path":"/users","scheme":"https"},"response":{"status":200},"startTime":"2024-12-30T10:00:00Z","durationMs":45.5}
```

### HAR

HTTP Archive format, compatible with browser DevTools. Use Ctrl+C to gracefully shutdown and write the HAR file.

### IR

APISpecRift Intermediate Representation format, for generating OpenAPI specs.

## Docker

### Dockerfile

The included multi-stage Dockerfile creates a minimal image:

```dockerfile
# Build
docker build -t omniproxy .

# Run with file output
docker run -p 8080:8080 -v $(pwd)/data:/data omniproxy serve \
  --host 0.0.0.0 \
  --output /data/traffic.ndjson

# Run with SQLite
docker run -p 8080:8080 -p 9090:9090 -v $(pwd)/data:/data omniproxy serve \
  --host 0.0.0.0 \
  --db sqlite:///data/traffic.db \
  --metrics-port 9090
```

### Docker Compose

```yaml
version: '3.8'
services:
  omniproxy:
    build: .
    ports:
      - "8080:8080"
      - "9090:9090"
    volumes:
      - ./data:/data
    command:
      - serve
      - --host=0.0.0.0
      - --db=sqlite:///data/traffic.db
      - --metrics-port=9090
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:9090/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
```

## Integration with APISpecRift

OmniProxy is designed to work with [APISpecRift](https://github.com/grokify/apispecrift) for generating OpenAPI specifications from captured traffic:

```bash
# Capture traffic
omniproxy serve --output traffic.ndjson --format ir

# Generate OpenAPI spec
apispecrift generate --input traffic.ndjson --output openapi.yaml
```

## Platform Support

| Platform | Proxy Config | CA Install |
|----------|--------------|------------|
| macOS    | networksetup | Keychain   |
| Windows  | Registry     | certutil   |
| Linux    | GNOME/KDE/env | ca-certificates |

## Security Considerations

- The CA private key is stored in `~/.omniproxy/ca/` with restricted permissions (0600)
- Sensitive headers (Authorization, Cookie, etc.) are filtered by default
- Use `--skip-host` for applications with certificate pinning
- Use filtering to capture only relevant traffic
- Database passwords in PostgreSQL URLs are hidden in logs

## License

MIT

 [build-status-svg]: https://github.com/grokify/omniproxy/actions/workflows/ci.yaml/badge.svg?branch=main
 [build-status-url]: https://github.com/grokify/omniproxy/actions/workflows/ci.yaml
 [lint-status-svg]: https://github.com/grokify/omniproxy/actions/workflows/lint.yaml/badge.svg?branch=main
 [lint-status-url]: https://github.com/grokify/omniproxy/actions/workflows/lint.yaml
 [goreport-svg]: https://goreportcard.com/badge/github.com/grokify/omniproxy
 [goreport-url]: https://goreportcard.com/report/github.com/grokify/omniproxy
 [docs-godoc-svg]: https://pkg.go.dev/badge/github.com/grokify/omniproxy
 [docs-godoc-url]: https://pkg.go.dev/github.com/grokify/omniproxy
 [viz-svg]: https://img.shields.io/badge/visualizaton-Go-blue.svg
 [viz-url]: https://mango-dune-07a8b7110.1.azurestaticapps.net/?repo=grokify%2Fomniproxy
 [loc-svg]: https://tokei.rs/b1/github/grokify/omniproxy
 [repo-url]: https://github.com/grokify/omniproxy
 [license-svg]: https://img.shields.io/badge/license-MIT-blue.svg
 [license-url]: https://github.com/grokify/omniproxy/blob/master/LICENSE
