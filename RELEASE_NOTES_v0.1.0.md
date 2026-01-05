# OmniProxy v0.1.0 Release Notes

**Release Date:** 2026-01-04

## Overview

OmniProxy v0.1.0 is the initial release of a universal HTTP/HTTPS proxy with MITM support for traffic capture and analysis. It supports three deployment modes (laptop, team, production) with the same binary.

## Highlights

- **Universal Proxy** - HTTP forward proxy with HTTPS MITM interception
- **Traffic Capture** - Export to NDJSON, HAR, or IR formats for API analysis
- **Database Storage** - SQLite for teams, PostgreSQL for production
- **Cross-Platform** - System proxy configuration for macOS, Windows, and Linux
- **Observability** - Prometheus metrics and health check endpoints
- **Pure Go** - No OpenSSL dependency, uses Go's crypto libraries

## Features

### Proxy Modes

| Mode | Storage | Use Case |
|------|---------|----------|
| **Laptop** | File (NDJSON) | Individual developer |
| **Team** | SQLite | Small team, shared server |
| **Production** | PostgreSQL | Enterprise, multi-tenant |

### Core Features

- **Forward Proxy** - HTTP proxy for routing traffic
- **MITM Proxy** - HTTPS interception with automatic certificate generation
- **Reverse Proxy** - Server-side proxy with Let's Encrypt/ACME support
- **Traffic Capture** - Export to NDJSON, HAR, or IR formats
- **Request Filtering** - Include/exclude by host, path, or method with wildcards
- **Binary Detection** - Automatically skip binary content (images, videos, etc.)
- **Proxy Chaining** - Forward through upstream proxy servers
- **Async Storage** - Configurable queue size, batch size, and worker count

### Observability

- Prometheus metrics endpoint (`/metrics`)
- Liveness probe (`/healthz`)
- Readiness probe (`/readyz`)

### CLI Commands

- `omniproxy serve` - Start the proxy server
- `omniproxy reverse` - Start reverse proxy with ACME TLS
- `omniproxy daemon` - Run as background daemon
- `omniproxy ca` - Manage CA certificates (generate, install, uninstall, info)
- `omniproxy system` - Manage system proxy settings (set, unset, status)
- `omniproxy config` - Manage configuration files (init, show)

## Installation

### From Source

```bash
go install github.com/grokify/omniproxy/cmd/omniproxy@v0.1.0
```

### Docker

```bash
docker pull ghcr.io/grokify/omniproxy:v0.1.0
```

## Quick Start

```bash
# Generate and install CA certificate
omniproxy ca generate
sudo omniproxy ca install

# Start proxy (laptop mode)
omniproxy serve --output traffic.ndjson

# Configure system to use proxy
omniproxy system set --port 8080
```

## Platform Support

| Platform | Proxy Config | CA Install |
|----------|--------------|------------|
| macOS | networksetup | Keychain |
| Windows | Registry | certutil |
| Linux | GNOME/KDE/env | ca-certificates |

## Documentation

- [README](README.md) - Full documentation with examples
- [CHANGELOG](CHANGELOG.md) - Version history

## Known Limitations

- Certificate pinned applications require `--skip-host` configuration
- Running reverse proxy on ports 80/443 requires root/sudo

## Contributors

- Initial release by [grokify](https://github.com/grokify)

## License

MIT License - see [LICENSE](LICENSE) for details.
