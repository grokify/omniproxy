package backend

import (
	"testing"
)

func TestParseDatabaseURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantType    DBType
		wantDriver  string
		wantDSN     string
		wantPath    string
		wantHost    string
		wantPort    string
		wantDB      string
		wantErr     bool
		errContains string
	}{
		{
			name:       "sqlite memory",
			url:        "sqlite::memory:",
			wantType:   DBTypeSQLite,
			wantDriver: "sqlite3",
			wantDSN:    ":memory:",
			wantPath:   ":memory:",
		},
		{
			name:       "sqlite memory alt",
			url:        "sqlite://:memory:",
			wantType:   DBTypeSQLite,
			wantDriver: "sqlite3",
			wantDSN:    ":memory:",
			wantPath:   ":memory:",
		},
		{
			name:       "sqlite relative path",
			url:        "sqlite://data.db",
			wantType:   DBTypeSQLite,
			wantDriver: "sqlite3",
			wantDSN:    "data.db",
			wantPath:   "data.db",
		},
		{
			name:       "sqlite relative path with dir",
			url:        "sqlite://./data/traffic.db",
			wantType:   DBTypeSQLite,
			wantDriver: "sqlite3",
			wantDSN:    "./data/traffic.db",
			wantPath:   "./data/traffic.db",
		},
		{
			name:       "sqlite absolute path",
			url:        "sqlite:///var/lib/omniproxy/data.db",
			wantType:   DBTypeSQLite,
			wantDriver: "sqlite3",
			wantDSN:    "/var/lib/omniproxy/data.db",
			wantPath:   "/var/lib/omniproxy/data.db",
		},
		{
			name:       "sqlite3 scheme",
			url:        "sqlite3://data.db",
			wantType:   DBTypeSQLite,
			wantDriver: "sqlite3",
			wantDSN:    "data.db",
			wantPath:   "data.db",
		},
		{
			name:       "sqlite with query params",
			url:        "sqlite://data.db?cache=shared&mode=rwc",
			wantType:   DBTypeSQLite,
			wantDriver: "sqlite3",
			wantDSN:    "data.db?cache=shared&mode=rwc",
			wantPath:   "data.db",
		},
		{
			name:       "postgres basic",
			url:        "postgres://localhost/omniproxy",
			wantType:   DBTypePostgres,
			wantDriver: "postgres",
			wantHost:   "localhost",
			wantPort:   "5432",
			wantDB:     "omniproxy",
		},
		{
			name:       "postgres with port",
			url:        "postgres://localhost:5433/omniproxy",
			wantType:   DBTypePostgres,
			wantDriver: "postgres",
			wantHost:   "localhost",
			wantPort:   "5433",
			wantDB:     "omniproxy",
		},
		{
			name:       "postgres with user",
			url:        "postgres://user@localhost/omniproxy",
			wantType:   DBTypePostgres,
			wantDriver: "postgres",
			wantHost:   "localhost",
			wantPort:   "5432",
			wantDB:     "omniproxy",
		},
		{
			name:       "postgres with user and password",
			url:        "postgres://user:pass@localhost/omniproxy",
			wantType:   DBTypePostgres,
			wantDriver: "postgres",
			wantHost:   "localhost",
			wantPort:   "5432",
			wantDB:     "omniproxy",
		},
		{
			name:       "postgres with sslmode",
			url:        "postgres://localhost/omniproxy?sslmode=disable",
			wantType:   DBTypePostgres,
			wantDriver: "postgres",
			wantHost:   "localhost",
			wantPort:   "5432",
			wantDB:     "omniproxy",
		},
		{
			name:       "postgresql scheme",
			url:        "postgresql://localhost/omniproxy",
			wantType:   DBTypePostgres,
			wantDriver: "postgres",
			wantHost:   "localhost",
			wantPort:   "5432",
			wantDB:     "omniproxy",
		},
		{
			name:        "empty url",
			url:         "",
			wantErr:     true,
			errContains: "required",
		},
		{
			name:        "unsupported scheme",
			url:         "mysql://localhost/db",
			wantErr:     true,
			errContains: "unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseDatabaseURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if cfg.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", cfg.Type, tt.wantType)
			}
			if cfg.DriverName != tt.wantDriver {
				t.Errorf("DriverName = %v, want %v", cfg.DriverName, tt.wantDriver)
			}

			if tt.wantType == DBTypeSQLite {
				if cfg.DSN != tt.wantDSN {
					t.Errorf("DSN = %v, want %v", cfg.DSN, tt.wantDSN)
				}
				if cfg.Path != tt.wantPath {
					t.Errorf("Path = %v, want %v", cfg.Path, tt.wantPath)
				}
			}

			if tt.wantType == DBTypePostgres {
				if cfg.Host != tt.wantHost {
					t.Errorf("Host = %v, want %v", cfg.Host, tt.wantHost)
				}
				if cfg.Port != tt.wantPort {
					t.Errorf("Port = %v, want %v", cfg.Port, tt.wantPort)
				}
				if cfg.Database != tt.wantDB {
					t.Errorf("Database = %v, want %v", cfg.Database, tt.wantDB)
				}
			}
		})
	}
}

func TestDBConfigString(t *testing.T) {
	tests := []struct {
		name string
		cfg  *DBConfig
		want string
	}{
		{
			name: "sqlite",
			cfg: &DBConfig{
				Type: DBTypeSQLite,
				Path: "/path/to/db.sqlite",
			},
			want: "sqlite:///path/to/db.sqlite",
		},
		{
			name: "postgres without user",
			cfg: &DBConfig{
				Type:     DBTypePostgres,
				Host:     "localhost",
				Port:     "5432",
				Database: "omniproxy",
			},
			want: "postgres://localhost:5432/omniproxy",
		},
		{
			name: "postgres with user (password hidden)",
			cfg: &DBConfig{
				Type:     DBTypePostgres,
				Host:     "localhost",
				Port:     "5432",
				Database: "omniproxy",
				User:     "admin",
				Password: "secret",
			},
			want: "postgres://admin:****@localhost:5432/omniproxy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
