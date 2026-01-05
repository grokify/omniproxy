// Package store provides database initialization and multi-tenant support.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/grokify/omniproxy/ui/ent"

	_ "github.com/lib/pq"           // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Config holds database configuration.
type Config struct {
	// Driver is the database driver: "sqlite3" or "postgres"
	Driver string

	// DSN is the data source name (connection string)
	// SQLite: "file:omniproxy.db?cache=shared&_fk=1"
	// PostgreSQL: "postgres://user:pass@host:5432/dbname?sslmode=disable"
	DSN string

	// EnableRLS enables PostgreSQL Row-Level Security for multi-tenancy
	// Only applies when Driver is "postgres"
	EnableRLS bool

	// Debug enables Ent debug logging
	Debug bool
}

// Store wraps the Ent client with multi-tenant support.
type Store struct {
	client    *ent.Client
	db        *sql.DB
	config    Config
	rlsHelper *RLSHelper
}

// New creates a new Store with the given configuration.
func New(cfg Config) (*Store, error) {
	var client *ent.Client
	var db *sql.DB
	var err error

	switch cfg.Driver {
	case "sqlite3", "sqlite":
		db, err = sql.Open("sqlite3", cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite: %w", err)
		}
		// SQLite optimizations
		if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
		}
		if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}
		drv := entsql.OpenDB(dialect.SQLite, db)
		opts := []ent.Option{ent.Driver(drv)}
		if cfg.Debug {
			opts = append(opts, ent.Debug())
		}
		client = ent.NewClient(opts...)

	case "postgres", "postgresql":
		db, err = sql.Open("postgres", cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres: %w", err)
		}
		drv := entsql.OpenDB(dialect.Postgres, db)
		opts := []ent.Option{ent.Driver(drv)}
		if cfg.Debug {
			opts = append(opts, ent.Debug())
		}
		client = ent.NewClient(opts...)

	default:
		return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
	}

	store := &Store{
		client: client,
		db:     db,
		config: cfg,
	}

	if cfg.Driver == "postgres" || cfg.Driver == "postgresql" {
		store.rlsHelper = NewRLSHelper(db)
	}

	return store, nil
}

// Client returns the Ent client.
func (s *Store) Client() *ent.Client {
	return s.client
}

// DB returns the underlying sql.DB connection.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Migrate runs database migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if err := s.client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Setup RLS if enabled and using PostgreSQL
	if s.config.EnableRLS && s.rlsHelper != nil {
		if err := s.rlsHelper.SetupRLS(ctx); err != nil {
			return fmt.Errorf("failed to setup RLS: %w", err)
		}
	}

	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if err := s.client.Close(); err != nil {
		return err
	}
	return s.db.Close()
}

// WithOrg returns a new context with the org ID set for RLS.
// This should be called at the start of each request to set tenant context.
func (s *Store) WithOrg(ctx context.Context, orgID int) context.Context {
	return context.WithValue(ctx, orgContextKey, orgID)
}

// OrgFromContext extracts the org ID from context.
func OrgFromContext(ctx context.Context) (int, bool) {
	orgID, ok := ctx.Value(orgContextKey).(int)
	return orgID, ok
}

type contextKey string

const orgContextKey contextKey = "org_id"

// TenantTx starts a transaction with RLS tenant context set.
// For PostgreSQL with RLS enabled, this sets the app.current_org_id variable.
// For SQLite, it returns a normal transaction (filtering must be done in queries).
func (s *Store) TenantTx(ctx context.Context, orgID int) (*ent.Tx, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}

	// Set RLS context for PostgreSQL
	if s.rlsHelper != nil && s.config.EnableRLS {
		if err := s.rlsHelper.SetTenantContext(ctx, orgID); err != nil {
			_ = tx.Rollback() // Ignore rollback error, already returning an error
			return nil, fmt.Errorf("failed to set tenant context: %w", err)
		}
	}

	return tx, nil
}

// DefaultSQLiteDSN returns a default SQLite connection string.
func DefaultSQLiteDSN(path string) string {
	return fmt.Sprintf("file:%s?cache=shared&_fk=1", path)
}

// DefaultPostgresDSN returns a default PostgreSQL connection string.
func DefaultPostgresDSN(host string, port int, user, password, dbname string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		user, password, host, port, dbname)
}
