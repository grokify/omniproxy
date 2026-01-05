// Package backend provides pluggable interfaces for OmniProxy storage and caching.
package backend

import (
	"fmt"
	"net/url"
	"strings"
)

// DBType represents a database type.
type DBType string

const (
	DBTypeSQLite   DBType = "sqlite"
	DBTypePostgres DBType = "postgres"
)

// DBConfig holds parsed database configuration.
type DBConfig struct {
	// Type is the database type (sqlite, postgres).
	Type DBType

	// DriverName is the driver name for sql.Open().
	DriverName string

	// DSN is the data source name for sql.Open().
	DSN string

	// Path is the file path (for SQLite only).
	Path string

	// Host is the database host (for PostgreSQL only).
	Host string

	// Port is the database port (for PostgreSQL only).
	Port string

	// Database is the database name (for PostgreSQL only).
	Database string

	// User is the database user (for PostgreSQL only).
	User string

	// Password is the database password (for PostgreSQL only).
	Password string

	// SSLMode is the SSL mode (for PostgreSQL only).
	SSLMode string
}

// ParseDatabaseURL parses a database URL into a DBConfig.
//
// Supported formats:
//   - sqlite://path/to/file.db
//   - sqlite:///absolute/path/to/file.db
//   - sqlite::memory: (in-memory database)
//   - postgres://user:password@host:port/database?sslmode=disable
//   - postgresql://user:password@host:port/database
func ParseDatabaseURL(rawURL string) (*DBConfig, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	// Handle SQLite memory database
	if rawURL == "sqlite::memory:" || rawURL == "sqlite://:memory:" {
		return &DBConfig{
			Type:       DBTypeSQLite,
			DriverName: "sqlite3",
			DSN:        ":memory:",
			Path:       ":memory:",
		}, nil
	}

	// Parse the URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}

	switch u.Scheme {
	case "sqlite", "sqlite3":
		return parseSQLiteURL(u)
	case "postgres", "postgresql":
		return parsePostgresURL(u, rawURL)
	default:
		return nil, fmt.Errorf("unsupported database scheme: %s (supported: sqlite, postgres)", u.Scheme)
	}
}

// parseSQLiteURL parses a SQLite database URL.
func parseSQLiteURL(u *url.URL) (*DBConfig, error) {
	// Get the path from the URL
	path := u.Host + u.Path
	if path == "" {
		path = u.Opaque
	}

	// Handle absolute paths (sqlite:///absolute/path)
	if u.Host == "" && strings.HasPrefix(u.Path, "/") {
		path = u.Path
	}

	if path == "" {
		return nil, fmt.Errorf("SQLite database path is required")
	}

	// Build DSN with query parameters
	dsn := path
	if u.RawQuery != "" {
		dsn += "?" + u.RawQuery
	}

	return &DBConfig{
		Type:       DBTypeSQLite,
		DriverName: "sqlite3",
		DSN:        dsn,
		Path:       path,
	}, nil
}

// parsePostgresURL parses a PostgreSQL database URL.
func parsePostgresURL(u *url.URL, rawURL string) (*DBConfig, error) {
	cfg := &DBConfig{
		Type:       DBTypePostgres,
		DriverName: "postgres",
	}

	// Extract host and port
	cfg.Host = u.Hostname()
	cfg.Port = u.Port()
	if cfg.Port == "" {
		cfg.Port = "5432"
	}

	// Extract database name
	cfg.Database = strings.TrimPrefix(u.Path, "/")
	if cfg.Database == "" {
		cfg.Database = "omniproxy"
	}

	// Extract user and password
	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	// Extract SSL mode from query
	cfg.SSLMode = u.Query().Get("sslmode")
	if cfg.SSLMode == "" {
		cfg.SSLMode = "prefer"
	}

	// Build the DSN - lib/pq accepts the URL format directly
	// but we normalize it to ensure consistency
	cfg.DSN = rawURL

	return cfg, nil
}

// String returns a safe string representation (without password).
func (c *DBConfig) String() string {
	switch c.Type {
	case DBTypeSQLite:
		return fmt.Sprintf("sqlite://%s", c.Path)
	case DBTypePostgres:
		if c.User != "" {
			return fmt.Sprintf("postgres://%s:****@%s:%s/%s", c.User, c.Host, c.Port, c.Database)
		}
		return fmt.Sprintf("postgres://%s:%s/%s", c.Host, c.Port, c.Database)
	default:
		return "unknown"
	}
}
