# OmniProxy Roadmap

## High Priority (Core Functionality)

- [x] **Reverse Proxy Mode** - Server-side proxy with Let's Encrypt/ACME support
- [x] **HAR Export** - Proper HAR 1.2 format output for browser DevTools compatibility
- [x] **Host/Path Filtering** - `--include-host`, `--exclude-host`, `--include-path`, `--exclude-path` flags
- [x] **Config File** - YAML configuration file support
- [x] **Proxy Chaining** - Forward to upstream proxy via `--upstream` flag
- [x] **Body Handling** - Truncate large bodies, detect and skip binary content
- [x] **Pluggable Backends** - Interfaces for TrafficStore, CertCache, ConfigStore
- [x] **Multi-tenant Database** - Ent schemas with PostgreSQL RLS support
- [x] **Authentication** - Local (bcrypt) and OAuth (Google, GitHub, OIDC)

## High Priority (Production Readiness)

- [x] **Integrate Backend into CLI** - `--db` flag with SQLite and PostgreSQL support
- [x] **OpenTelemetry Instrumentation** - Prometheus metrics via OTel SDK
- [x] **Health Endpoints** - `/healthz` (liveness) and `/readyz` (readiness)
- [x] **Dockerfile** - Multi-stage build with Alpine runtime
- [x] **GitHub Actions CI** - Test, lint, build, Docker, security scanning (Gosec, Trivy)

## Medium Priority (Enhanced Capture)

- [ ] **WebSocket Support** - Capture WebSocket connection upgrades and messages
- [ ] **HTTP/2 Support** - Handle HTTP/2 connections and multiplexed streams
- [ ] **gRPC Support** - Capture gRPC traffic for API specification generation
- [ ] **Request Replay** - Replay captured requests from HAR/IR files

## Medium Priority (Production Backends)

- [ ] **Redis Cert Cache** - Distributed certificate cache for horizontal scaling
- [ ] **Kafka Traffic Store** - Async traffic pipeline for high volume
- [ ] **Goreleaser** - Cross-platform binary releases (macOS, Windows, Linux)
- [ ] **Homebrew Formula** - Easy installation on macOS

## Lower Priority (Power Features)

- [ ] **Request Modification** - Rewrite headers, body, URLs on the fly
- [ ] **Breakpoints** - Pause and inspect/modify requests before forwarding
- [ ] **Rate Limiting** - Throttle connections for testing slow network conditions
- [ ] **Proxy Authentication** - Require authentication to use the proxy

## Infrastructure

- [ ] **Documentation Site** - Comprehensive docs with examples

## Desktop UI

The desktop UI is implemented in [PlexusDesktop](https://github.com/plexusone/plexusdesktop), a native macOS app (Swift/SwiftUI).

See [TRD.md](./TRD.md) for architecture details.

- [x] **Menu Bar Control** - NSStatusItem with dynamic icon, popover for quick actions
- [x] **Traffic Inspector** - `/traffic` endpoint + SwiftUI list with filtering
- [x] **System Proxy Toggle** - Enable/disable macOS system proxy from UI
- [x] **Unix Socket IPC** - Swift client for daemon communication
- [x] **Bundle omniproxyd** - Makefile builds Go binary into `.app/Contents/MacOS/`
- [x] **Request Detail View** - Split view with headers/body, `/traffic/{id}` endpoint
- [x] **Auto-update** - Sparkle framework for automatic updates
- [x] **Code signing** - Makefile with codesign + notarization support (see below)

### Apple Developer ID Signing

The Makefile supports full code signing and notarization, but PlexusDesktop is **not currently signed with an Apple Developer ID**. Users must bypass Gatekeeper on first run:

```bash
xattr -cr /Applications/PlexusDesktop.app
```

**Options for Apple-signed distribution:**

| Approach | Cost | Pros | Cons |
|----------|------|------|------|
| **Unsigned (current)** | Free | No cost, no personal info exposed | Users must bypass Gatekeeper |
| **Homebrew Cask** | Free | Homebrew handles quarantine removal | Requires Homebrew, no DMG |
| **Developer ID (personal)** | $99/yr | Full Gatekeeper approval | Maintainer's name on cert |
| **Developer ID (org)** | $99/yr | Org name on cert, professional | Requires legal entity (LLC, etc.) |
| **GitHub Sponsors funding** | $99/yr | Community funded, transparent | Need sufficient sponsors |

**When to consider signing:**
- User adoption grows significantly
- Enterprise users require signed apps
- Project receives sponsorship/funding
- Maintainer creates org entity for PlexusOne projects

**To sign with Developer ID:**
```bash
# Store notarization credentials (one-time)
xcrun notarytool store-credentials "plexusdesktop" \
  --apple-id "developer@example.com" \
  --team-id "TEAMID"

# Build, sign, and notarize
SIGNING_IDENTITY="Developer ID Application: Name (TEAMID)" make release
```

## Completed

- [x] Forward proxy with MITM support
- [x] Pure Go CA certificate generation
- [x] Traffic capture to NDJSON/JSON/HAR/IR formats
- [x] System proxy configuration (macOS/Windows/Linux)
- [x] CLI with cobra
- [x] Sensitive header filtering
- [x] Basic test coverage
- [x] Binary content detection (contentdetect package)
- [x] Async traffic store with batching
- [x] Traffic sampling for high volume
- [x] Memory cert cache (TTL and LRU variants)
- [x] Daemon mode with Unix socket control API (`omniproxy daemon start/stop/status/reload`)
- [x] PID file management and signal handling

## Deployment Modes

OmniProxy supports three deployment modes with the same binary:

| Mode | Database | Cert Cache | Traffic Store | Use Case |
|------|----------|------------|---------------|----------|
| **Laptop** | None | Memory | File (NDJSON) | Individual developer |
| **Team** | SQLite | Memory | SQLite | Small team, shared UI |
| **Production** | PostgreSQL | Redis | Kafka → ClickHouse | Enterprise scale |

For production infrastructure (Helm, Terraform, etc.), see [PlexusProxy](https://github.com/plexusone/plexusproxy).
