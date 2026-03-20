package pgutil

import "testing"

func TestDatabaseConfigGetConnectionStringAddsSSLModeWhenMissing(t *testing.T) {
	cfg := &DatabaseConfig{
		URL:     "postgres://user:pass@localhost:5432/dbname",
		SSLMode: "disable",
	}

	got := cfg.GetConnectionString()
	want := "postgres://user:pass@localhost:5432/dbname?sslmode=disable"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDatabaseConfigGetConnectionStringPreservesExistingSSLMode(t *testing.T) {
	cfg := &DatabaseConfig{
		URL:     "postgres://user:pass@localhost:5432/dbname?sslmode=require",
		SSLMode: "disable",
	}

	got := cfg.GetConnectionString()
	want := "postgres://user:pass@localhost:5432/dbname?sslmode=require"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDatabaseConfigGetConnectionString(t *testing.T) {
	cfg := &DatabaseConfig{
		URL:     "postgres://user:pass@localhost:5432/dbname",
		SSLMode: "disable",
	}

	got := cfg.GetConnectionString()
	want := "postgres://user:pass@localhost:5432/dbname?sslmode=disable"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDatabaseConfigGetConnectionStringNilReceiver(t *testing.T) {
	var cfg *DatabaseConfig
	if got := cfg.GetConnectionString(); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
