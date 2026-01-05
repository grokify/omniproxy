# OmniProxy - Technical Requirements Document

## Overview

OmniProxy is a universal HTTP/HTTPS proxy with MITM support for traffic capture and API specification generation. This document covers the technical architecture for the CLI/daemon.

**Note:** The desktop UI is implemented in [PlexusDesktop](https://github.com/plexusone/plexusdesktop), a separate Swift/SwiftUI macOS application that communicates with `omniproxyd` via Unix socket.

---

## Repository Structure

| Repository | Language | Purpose |
|------------|----------|---------|
| `github.com/grokify/omniproxy` | **Go** | CLI + daemon (this repo) |
| `github.com/plexusone/plexusdesktop` | **Swift** | macOS UI for PlexusOne tools |

**This repo contains Go code only.** The Swift UI lives in PlexusDesktop.

### How They Work Together

1. **OmniProxy** (Go) provides:
   - `omniproxy` CLI for standalone use
   - `omniproxy daemon start` for background operation
   - Unix socket API at `~/.omniproxy/omniproxyd.sock`

2. **PlexusDesktop** (Swift) provides:
   - macOS menu bar app
   - Traffic inspector UI
   - Bundles `omniproxyd` binary in the `.app`
   - Communicates via Unix socket IPC

### Build & Distribution

```
# Standalone (Go only)
go install github.com/grokify/omniproxy/cmd/omniproxy@latest
omniproxy serve --port 8080

# With PlexusDesktop (Swift bundles Go binary)
PlexusDesktop.app/
└── Contents/
    └── MacOS/
        ├── PlexusDesktop    # Swift binary
        └── omniproxyd       # Go binary (bundled at build time)
```

---

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                        OmniProxy UI                              │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │              macOS App (Swift/SwiftUI)                    │  │
│  │  • Menu bar icon with status                              │  │
│  │  • Traffic inspector window                               │  │
│  │  • Proxy configuration                                    │  │
│  │  • System proxy toggle                                    │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                   │
│                    Unix Socket IPC                               │
│                              │                                   │
│  ┌───────────────────────────▼───────────────────────────────┐  │
│  │                    omniproxyd (Go)                        │  │
│  │  • HTTP/HTTPS proxy server                                │  │
│  │  • MITM certificate generation                            │  │
│  │  • Traffic capture and storage                            │  │
│  │  • Control API on Unix socket                             │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                   │
│  ┌───────────────────────────▼───────────────────────────────┐  │
│  │                      Storage                              │  │
│  │  • SQLite (laptop/team mode)                              │  │
│  │  • PostgreSQL (production mode)                           │  │
│  │  • File (NDJSON/HAR for export)                           │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Daemon (omniproxyd)

The Go daemon handles all proxy functionality:

| Component | Description |
|-----------|-------------|
| **Proxy Server** | HTTP/HTTPS forward proxy with MITM |
| **CA Management** | Pure Go certificate generation |
| **Traffic Capture** | Request/response recording |
| **Control API** | Unix socket for UI communication |
| **Observability** | Prometheus metrics, health endpoints |

**Control API Endpoints** (Unix Socket):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/status` | GET | Daemon status (running, uptime, requests) |
| `/stop` | POST | Graceful shutdown |
| `/reload` | POST | Reload configuration |
| `/stats` | GET | Traffic statistics |
| `/config` | GET/POST | Get/set configuration |

---

## Desktop UI Options

### Option 1: Native Swift (macOS) - **Recommended**

Build a native macOS application using Swift and SwiftUI.

**Rationale:**
- OmniProxy is a developer tool; most developers use macOS
- Menu bar apps require native integration for best UX
- System proxy configuration is cleaner with native APIs
- Precedent: Proxyman, Charles Proxy (both native)
- Smallest binary size (~5-10MB)
- Best performance and memory usage

**Technology Stack:**

| Component | Technology |
|-----------|------------|
| Language | Swift 5.9+ |
| UI Framework | SwiftUI + AppKit |
| Menu Bar | NSStatusItem |
| IPC | Unix socket (URLSessionStreamTask) |
| Data | SwiftData or Core Data |
| Build | Xcode 15+ |

**App Structure:**

```
OmniProxyApp/
├── OmniProxyApp.xcodeproj
├── OmniProxyApp/
│   ├── App/
│   │   ├── OmniProxyApp.swift        # App entry point
│   │   ├── AppDelegate.swift         # NSApplicationDelegate
│   │   └── MenuBarController.swift   # NSStatusItem management
│   │
│   ├── Views/
│   │   ├── MenuBarView.swift         # Menu bar popup
│   │   ├── MainWindow/
│   │   │   ├── MainWindowView.swift  # Traffic inspector
│   │   │   ├── RequestListView.swift # Request list sidebar
│   │   │   ├── RequestDetailView.swift
│   │   │   └── FilterBar.swift
│   │   ├── Settings/
│   │   │   ├── SettingsView.swift
│   │   │   ├── ProxySettings.swift
│   │   │   └── CertificateSettings.swift
│   │   └── Onboarding/
│   │       └── CAInstallView.swift
│   │
│   ├── Services/
│   │   ├── DaemonClient.swift        # Unix socket communication
│   │   ├── ProxyManager.swift        # Start/stop/status
│   │   ├── SystemProxyManager.swift  # macOS proxy settings
│   │   └── CertificateManager.swift  # Keychain integration
│   │
│   ├── Models/
│   │   ├── ProxyStatus.swift
│   │   ├── TrafficRecord.swift
│   │   └── ProxyConfig.swift
│   │
│   ├── Utilities/
│   │   ├── Keychain.swift
│   │   └── ProcessRunner.swift       # Run omniproxy CLI
│   │
│   └── Resources/
│       ├── Assets.xcassets
│       └── Info.plist
│
└── OmniProxyAppTests/
```

**Key Features:**

1. **Menu Bar App**
   - Status icon (green/red for running/stopped)
   - Quick toggle for proxy on/off
   - Quick toggle for system proxy
   - Recent requests preview
   - Open main window

2. **Main Window**
   - Traffic inspector (like Charles/Proxyman)
   - Request/response viewer
   - Filtering by host, method, status
   - Search
   - Export to HAR

3. **Settings**
   - Proxy port configuration
   - MITM enable/disable
   - Host filtering
   - Certificate management

**Platform Requirements:**
- macOS 13.0+ (Ventura)
- Apple Silicon and Intel support

---

### Option 2: Wails v2 (Cross-Platform) - **Fallback**

If Swift development proves too time-consuming, fall back to Wails.

**Technology Stack:**

| Component | Technology |
|-----------|------------|
| Framework | Wails v2 |
| Backend | Go |
| Frontend | React 18 + TypeScript |
| Styling | Tailwind CSS + Shadcn/ui |
| Build | Vite |

**Rationale:**
- Cross-platform (macOS, Windows, Linux)
- Reuse Go code from daemon
- Faster development if familiar with React
- ~15MB binary (uses native webview, not Electron)

**Trade-offs:**
- Menu bar integration less native
- System proxy management requires CLI calls
- WebView has some rendering quirks

**When to Fall Back:**
- Swift/SwiftUI learning curve too steep
- Need Windows/Linux support sooner
- Team more comfortable with React

---

### Option 3: Electron - **Not Recommended**

| Aspect | Issue |
|--------|-------|
| Binary Size | ~150MB |
| Memory Usage | ~200MB+ |
| Native Feel | Poor menu bar integration |
| Justification | Only if existing Electron expertise |

---

## Decision: Native Swift First

**Primary:** Native Swift/SwiftUI for macOS

**Fallback:** Wails v2 + React if:
- Swift development takes > 2x estimated time
- Critical bugs in Swift implementation
- Immediate need for Windows/Linux support

---

## macOS Integration Details

### Menu Bar (NSStatusItem)

```swift
class MenuBarController {
    private var statusItem: NSStatusItem?

    func setup() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem?.button?.image = NSImage(named: "MenuBarIcon")

        let menu = NSMenu()
        menu.addItem(NSMenuItem(title: "Status: Running", action: nil, keyEquivalent: ""))
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Toggle Proxy", action: #selector(toggleProxy), keyEquivalent: "p"))
        menu.addItem(NSMenuItem(title: "Toggle System Proxy", action: #selector(toggleSystemProxy), keyEquivalent: "s"))
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Open Inspector...", action: #selector(openInspector), keyEquivalent: "i"))
        menu.addItem(NSMenuItem(title: "Settings...", action: #selector(openSettings), keyEquivalent: ","))
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Quit", action: #selector(quit), keyEquivalent: "q"))

        statusItem?.menu = menu
    }
}
```

### System Proxy Configuration

```swift
import SystemConfiguration

class SystemProxyManager {
    func setHTTPProxy(host: String, port: Int) throws {
        // Use networksetup CLI for reliability
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/sbin/networksetup")
        process.arguments = ["-setwebproxy", "Wi-Fi", host, String(port)]
        try process.run()
        process.waitUntilExit()
    }

    func setHTTPSProxy(host: String, port: Int) throws {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/sbin/networksetup")
        process.arguments = ["-setsecurewebproxy", "Wi-Fi", host, String(port)]
        try process.run()
        process.waitUntilExit()
    }

    func disableProxy() throws {
        // Disable both HTTP and HTTPS proxy
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/sbin/networksetup")
        process.arguments = ["-setwebproxystate", "Wi-Fi", "off"]
        try process.run()
        process.waitUntilExit()

        let process2 = Process()
        process2.executableURL = URL(fileURLWithPath: "/usr/sbin/networksetup")
        process2.arguments = ["-setsecurewebproxystate", "Wi-Fi", "off"]
        try process2.run()
        process2.waitUntilExit()
    }
}
```

### Unix Socket Communication

```swift
import Foundation

class DaemonClient {
    private let socketPath: String

    init(socketPath: String = "~/.omniproxy/omniproxyd.sock") {
        self.socketPath = NSString(string: socketPath).expandingTildeInPath
    }

    func getStatus() async throws -> ProxyStatus {
        let data = try await request(path: "/status", method: "GET")
        return try JSONDecoder().decode(ProxyStatus.self, from: data)
    }

    func stop() async throws {
        _ = try await request(path: "/stop", method: "POST")
    }

    func reload() async throws {
        _ = try await request(path: "/reload", method: "POST")
    }

    private func request(path: String, method: String, body: Data? = nil) async throws -> Data {
        // Create Unix socket connection
        let socket = try Socket(path: socketPath)
        defer { socket.close() }

        // Build HTTP request
        var request = "\\(method) \\(path) HTTP/1.1\\r\\n"
        request += "Host: localhost\\r\\n"
        request += "Connection: close\\r\\n"
        if let body = body {
            request += "Content-Length: \\(body.count)\\r\\n"
            request += "Content-Type: application/json\\r\\n"
        }
        request += "\\r\\n"

        try socket.write(request.data(using: .utf8)!)
        if let body = body {
            try socket.write(body)
        }

        // Read response
        let response = try socket.readAll()

        // Parse HTTP response (simple parser)
        guard let bodyStart = response.range(of: "\\r\\n\\r\\n".data(using: .utf8)!) else {
            throw DaemonError.invalidResponse
        }

        return response.suffix(from: bodyStart.upperBound)
    }
}
```

### Keychain Integration (CA Certificate)

```swift
import Security

class CertificateManager {
    func installCA(certificatePath: String) throws {
        let certURL = URL(fileURLWithPath: certificatePath)
        let certData = try Data(contentsOf: certURL)

        guard let certificate = SecCertificateCreateWithData(nil, certData as CFData) else {
            throw CertificateError.invalidCertificate
        }

        let addQuery: [String: Any] = [
            kSecClass as String: kSecClassCertificate,
            kSecValueRef as String: certificate,
            kSecAttrLabel as String: "OmniProxy CA"
        ]

        let status = SecItemAdd(addQuery as CFDictionary, nil)
        guard status == errSecSuccess || status == errSecDuplicateItem else {
            throw CertificateError.keychainError(status)
        }

        // Trust the certificate
        try trustCertificate(certificate)
    }

    func isCAInstalled() -> Bool {
        let query: [String: Any] = [
            kSecClass as String: kSecClassCertificate,
            kSecAttrLabel as String: "OmniProxy CA",
            kSecReturnRef as String: true
        ]

        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        return status == errSecSuccess
    }
}
```

---

## Distribution

### macOS App

| Aspect | Details |
|--------|---------|
| **Signing** | Developer ID Application certificate |
| **Notarization** | Required for distribution outside App Store |
| **Distribution** | DMG via GitHub Releases, Homebrew Cask |
| **Auto-Update** | Sparkle framework |

### Daemon (omniproxyd)

| Aspect | Details |
|--------|---------|
| **Distribution** | Bundled in .app or separate Homebrew formula |
| **Installation** | `brew install omniproxy` or bundled |
| **Location** | `/usr/local/bin/omniproxy` or in .app bundle |

---

## Development Phases

### Phase 1: Menu Bar MVP
- [ ] Basic menu bar app with status icon
- [ ] Start/stop daemon control
- [ ] System proxy toggle
- [ ] Status display (running, port, requests)

### Phase 2: Traffic Inspector
- [ ] Main window with request list
- [ ] Request/response detail view
- [ ] Basic filtering (host, method)
- [ ] Search

### Phase 3: Configuration
- [ ] Settings window
- [ ] Proxy port configuration
- [ ] Host include/exclude lists
- [ ] CA certificate installation wizard

### Phase 4: Polish
- [ ] Onboarding flow
- [ ] Auto-update (Sparkle)
- [ ] Code signing and notarization
- [ ] Homebrew Cask formula

---

## Summary

| Decision | Choice |
|----------|--------|
| **Primary UI** | Native Swift/SwiftUI for macOS |
| **Fallback** | Wails v2 + React |
| **Daemon** | Go (omniproxyd) |
| **IPC** | Unix socket with HTTP API |
| **Distribution** | DMG + Homebrew Cask |
