package backend

import (
	"context"
	"testing"
	"time"

	"github.com/grokify/omniproxy/pkg/capture"
)

func TestDatabaseTrafficStore(t *testing.T) {
	ctx := context.Background()

	// Create an in-memory SQLite database
	store, err := NewDatabaseTrafficStore(ctx, &DatabaseTrafficStoreConfig{
		DatabaseURL: "sqlite::memory:",
		ProxyName:   "test-proxy",
	})
	if err != nil {
		t.Fatalf("failed to create database store: %v", err)
	}
	defer store.Close()

	// Test that proxy was created
	if store.ProxyID() == 0 {
		t.Error("expected non-zero proxy ID")
	}

	// Test Store
	t.Run("Store", func(t *testing.T) {
		rec := &capture.Record{
			StartTime: time.Now(),
			EndTime:   time.Now().Add(100 * time.Millisecond),
			DurationMs: 100.0,
			Request: capture.RequestRecord{
				Method:      "GET",
				URL:         "https://example.com/api/test",
				Host:        "example.com",
				Path:        "/api/test",
				Scheme:      "https",
				Headers:     map[string]string{"content-type": "application/json"},
				ContentType: "application/json",
			},
			Response: capture.ResponseRecord{
				Status:     200,
				StatusText: "200 OK",
				Headers:    map[string]string{"content-type": "application/json"},
			},
		}

		err := store.Store(ctx, rec)
		if err != nil {
			t.Errorf("Store failed: %v", err)
		}
	})

	// Test StoreBatch
	t.Run("StoreBatch", func(t *testing.T) {
		recs := []*capture.Record{
			{
				StartTime:  time.Now(),
				DurationMs: 50.0,
				Request: capture.RequestRecord{
					Method: "POST",
					URL:    "https://example.com/api/create",
					Host:   "example.com",
					Path:   "/api/create",
					Scheme: "https",
				},
				Response: capture.ResponseRecord{
					Status: 201,
				},
			},
			{
				StartTime:  time.Now(),
				DurationMs: 75.0,
				Request: capture.RequestRecord{
					Method: "DELETE",
					URL:    "https://example.com/api/delete/1",
					Host:   "example.com",
					Path:   "/api/delete/1",
					Scheme: "https",
				},
				Response: capture.ResponseRecord{
					Status: 204,
				},
			},
		}

		err := store.StoreBatch(ctx, recs)
		if err != nil {
			t.Errorf("StoreBatch failed: %v", err)
		}
	})

	// Test Query
	t.Run("Query", func(t *testing.T) {
		records, err := store.Query(ctx, &TrafficFilter{
			Limit: 10,
		})
		if err != nil {
			t.Errorf("Query failed: %v", err)
		}
		if len(records) != 3 {
			t.Errorf("expected 3 records, got %d", len(records))
		}
	})

	// Test Query with filter
	t.Run("QueryWithFilter", func(t *testing.T) {
		records, err := store.Query(ctx, &TrafficFilter{
			Methods: []string{"GET"},
			Limit:   10,
		})
		if err != nil {
			t.Errorf("Query failed: %v", err)
		}
		if len(records) != 1 {
			t.Errorf("expected 1 GET record, got %d", len(records))
		}
	})

	// Test Count
	t.Run("Count", func(t *testing.T) {
		count, err := store.Count(ctx, nil)
		if err != nil {
			t.Errorf("Count failed: %v", err)
		}
		if count != 3 {
			t.Errorf("expected count 3, got %d", count)
		}
	})

	// Test Stats
	t.Run("Stats", func(t *testing.T) {
		stats, err := store.Stats(ctx, nil)
		if err != nil {
			t.Errorf("Stats failed: %v", err)
		}
		if stats.TotalRequests != 3 {
			t.Errorf("expected 3 total requests, got %d", stats.TotalRequests)
		}
		if stats.RequestsByMethod["GET"] != 1 {
			t.Errorf("expected 1 GET request, got %d", stats.RequestsByMethod["GET"])
		}
		if stats.RequestsByMethod["POST"] != 1 {
			t.Errorf("expected 1 POST request, got %d", stats.RequestsByMethod["POST"])
		}
	})
}

func TestDatabaseTrafficStoreNilRecord(t *testing.T) {
	ctx := context.Background()

	store, err := NewDatabaseTrafficStore(ctx, &DatabaseTrafficStoreConfig{
		DatabaseURL: "sqlite::memory:",
		ProxyName:   "test-proxy",
	})
	if err != nil {
		t.Fatalf("failed to create database store: %v", err)
	}
	defer store.Close()

	// Storing nil should not error
	err = store.Store(ctx, nil)
	if err != nil {
		t.Errorf("Store(nil) should not error: %v", err)
	}
}

func TestDatabaseTrafficStoreClose(t *testing.T) {
	ctx := context.Background()

	store, err := NewDatabaseTrafficStore(ctx, &DatabaseTrafficStoreConfig{
		DatabaseURL: "sqlite::memory:",
		ProxyName:   "test-proxy",
	})
	if err != nil {
		t.Fatalf("failed to create database store: %v", err)
	}

	// Close the store
	err = store.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail
	rec := &capture.Record{
		StartTime: time.Now(),
		Request: capture.RequestRecord{
			Method: "GET",
			URL:    "https://example.com",
			Host:   "example.com",
			Path:   "/",
			Scheme: "https",
		},
		Response: capture.ResponseRecord{
			Status: 200,
		},
	}

	err = store.Store(ctx, rec)
	if err == nil {
		t.Error("expected error storing after close")
	}
}

func TestDatabaseTrafficStoreInvalidURL(t *testing.T) {
	ctx := context.Background()

	_, err := NewDatabaseTrafficStore(ctx, &DatabaseTrafficStoreConfig{
		DatabaseURL: "invalid://url",
	})
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestDatabaseTrafficStoreQueryWithStatusFilter(t *testing.T) {
	ctx := context.Background()

	store, err := NewDatabaseTrafficStore(ctx, &DatabaseTrafficStoreConfig{
		DatabaseURL: "sqlite::memory:",
		ProxyName:   "test-proxy",
	})
	if err != nil {
		t.Fatalf("failed to create database store: %v", err)
	}
	defer store.Close()

	// Store records with different status codes
	records := []*capture.Record{
		{
			StartTime: time.Now(),
			Request: capture.RequestRecord{
				Method: "GET", URL: "https://example.com/ok", Host: "example.com", Path: "/ok", Scheme: "https",
			},
			Response: capture.ResponseRecord{Status: 200},
		},
		{
			StartTime: time.Now(),
			Request: capture.RequestRecord{
				Method: "GET", URL: "https://example.com/notfound", Host: "example.com", Path: "/notfound", Scheme: "https",
			},
			Response: capture.ResponseRecord{Status: 404},
		},
		{
			StartTime: time.Now(),
			Request: capture.RequestRecord{
				Method: "GET", URL: "https://example.com/error", Host: "example.com", Path: "/error", Scheme: "https",
			},
			Response: capture.ResponseRecord{Status: 500},
		},
	}

	for _, rec := range records {
		if err := store.Store(ctx, rec); err != nil {
			t.Fatalf("failed to store record: %v", err)
		}
	}

	// Query for errors (status >= 400)
	errorRecords, err := store.Query(ctx, &TrafficFilter{
		MinStatus: 400,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(errorRecords) != 2 {
		t.Errorf("expected 2 error records (404 and 500), got %d", len(errorRecords))
	}

	// Query for specific status
	notFoundRecords, err := store.Query(ctx, &TrafficFilter{
		StatusCodes: []int{404},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(notFoundRecords) != 1 {
		t.Errorf("expected 1 404 record, got %d", len(notFoundRecords))
	}
}
