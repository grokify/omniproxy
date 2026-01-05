package store

import (
	"context"
	"database/sql"
	"fmt"
)

// RLSHelper manages PostgreSQL Row-Level Security for multi-tenancy.
type RLSHelper struct {
	db *sql.DB
}

// NewRLSHelper creates a new RLS helper.
func NewRLSHelper(db *sql.DB) *RLSHelper {
	return &RLSHelper{db: db}
}

// SetupRLS creates RLS policies for all tenant-scoped tables.
// This should be called after schema migration.
func (r *RLSHelper) SetupRLS(ctx context.Context) error {
	// Tables that need RLS policies with their org reference
	tables := []struct {
		name      string
		orgColumn string // Column that references org, empty if table IS the org table
	}{
		{"users", "org_users"},       // users.org_users is the FK to orgs
		{"sessions", ""},             // sessions -> users -> orgs (indirect)
		{"proxies", "org_proxies"},   // proxies.org_proxies is the FK to orgs
		{"traffic", "proxy_traffic"}, // traffic -> proxies -> orgs (indirect)
	}

	for _, t := range tables {
		if err := r.setupTableRLS(ctx, t.name, t.orgColumn); err != nil {
			return fmt.Errorf("failed to setup RLS for %s: %w", t.name, err)
		}
	}

	return nil
}

// setupTableRLS enables RLS on a table and creates policies.
func (r *RLSHelper) setupTableRLS(ctx context.Context, tableName, orgColumn string) error {
	// Enable RLS on the table
	enableRLS := fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY", tableName)
	if _, err := r.db.ExecContext(ctx, enableRLS); err != nil {
		return fmt.Errorf("failed to enable RLS: %w", err)
	}

	// Force RLS for table owner too (important for security)
	forceRLS := fmt.Sprintf("ALTER TABLE %s FORCE ROW LEVEL SECURITY", tableName)
	if _, err := r.db.ExecContext(ctx, forceRLS); err != nil {
		return fmt.Errorf("failed to force RLS: %w", err)
	}

	// Drop existing policy if it exists
	dropPolicy := fmt.Sprintf("DROP POLICY IF EXISTS tenant_isolation ON %s", tableName)
	if _, err := r.db.ExecContext(ctx, dropPolicy); err != nil {
		return fmt.Errorf("failed to drop existing policy: %w", err)
	}

	// Create the appropriate policy based on table structure
	var createPolicy string
	switch tableName {
	case "users":
		// Users are directly linked to orgs
		createPolicy = fmt.Sprintf(`
			CREATE POLICY tenant_isolation ON %s
			FOR ALL
			USING (%s = current_setting('app.current_org_id')::int)
			WITH CHECK (%s = current_setting('app.current_org_id')::int)
		`, tableName, orgColumn, orgColumn)

	case "sessions":
		// Sessions are linked through users
		createPolicy = fmt.Sprintf(`
			CREATE POLICY tenant_isolation ON %s
			FOR ALL
			USING (
				user_sessions IN (
					SELECT id FROM users
					WHERE org_users = current_setting('app.current_org_id')::int
				)
			)
			WITH CHECK (
				user_sessions IN (
					SELECT id FROM users
					WHERE org_users = current_setting('app.current_org_id')::int
				)
			)
		`, tableName)

	case "proxies":
		// Proxies are directly linked to orgs
		createPolicy = fmt.Sprintf(`
			CREATE POLICY tenant_isolation ON %s
			FOR ALL
			USING (%s = current_setting('app.current_org_id')::int)
			WITH CHECK (%s = current_setting('app.current_org_id')::int)
		`, tableName, orgColumn, orgColumn)

	case "traffic":
		// Traffic is linked through proxies
		createPolicy = fmt.Sprintf(`
			CREATE POLICY tenant_isolation ON %s
			FOR ALL
			USING (
				%s IN (
					SELECT id FROM proxies
					WHERE org_proxies = current_setting('app.current_org_id')::int
				)
			)
			WITH CHECK (
				%s IN (
					SELECT id FROM proxies
					WHERE org_proxies = current_setting('app.current_org_id')::int
				)
			)
		`, tableName, orgColumn, orgColumn)

	default:
		return fmt.Errorf("unknown table: %s", tableName)
	}

	if _, err := r.db.ExecContext(ctx, createPolicy); err != nil {
		return fmt.Errorf("failed to create policy: %w", err)
	}

	return nil
}

// SetTenantContext sets the current tenant for RLS policies.
// This must be called at the start of each request/transaction.
func (r *RLSHelper) SetTenantContext(ctx context.Context, orgID int) error {
	query := fmt.Sprintf("SET app.current_org_id = '%d'", orgID)
	_, err := r.db.ExecContext(ctx, query)
	return err
}

// SetTenantContextTx sets the current tenant within a transaction.
func (r *RLSHelper) SetTenantContextTx(ctx context.Context, tx *sql.Tx, orgID int) error {
	query := fmt.Sprintf("SET LOCAL app.current_org_id = '%d'", orgID)
	_, err := tx.ExecContext(ctx, query)
	return err
}

// ClearTenantContext clears the tenant context.
func (r *RLSHelper) ClearTenantContext(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "RESET app.current_org_id")
	return err
}

// CreateBypassRole creates a role that bypasses RLS (for admin/system operations).
func (r *RLSHelper) CreateBypassRole(ctx context.Context, roleName string) error {
	// Create role if not exists
	createRole := fmt.Sprintf(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%s') THEN
				CREATE ROLE %s NOLOGIN BYPASSRLS;
			END IF;
		END
		$$;
	`, roleName, roleName)

	_, err := r.db.ExecContext(ctx, createRole)
	return err
}
