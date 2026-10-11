package store

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/testutil"
	"github.com/google/uuid"
)

func TestHTTPCompatibilityCheckIsReadOnlyAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	s := &Store{DB: testutil.Database(t)}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("options", "-c default_transaction_read_only=on")
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	readOnly := &Store{DB: db}
	var isReadOnly bool
	if err := db.QueryRow(`SELECT current_setting('transaction_read_only')='on'`).Scan(&isReadOnly); err != nil || !isReadOnly {
		t.Fatalf("read-only probe did not use read-only transactions: %v %v", isReadOnly, err)
	}
	if err := readOnly.CheckSchema(ctx); err != nil {
		t.Fatalf("HTTP schema check must work in read-only transactions: %v", err)
	}
}

func TestAuthMigrationCollisionRollsBackDDL(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	s := &Store{DB: db}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	testutil.ResetAccounts(t, db)
	t.Cleanup(func() {
		testutil.ResetAccounts(t, db)
		if err := s.Migrate(ctx); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.Exec(`DROP INDEX auth.users_username_lower_key; DROP TABLE auth.login_failures`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"LegacyConflict", "legacyconflict"} {
		if _, err := db.Exec(`INSERT INTO auth.users(id,username,password_hash) VALUES($1,$2,'unused')`, uuid.NewString(), name); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Migrate(ctx); err == nil || !strings.Contains(err.Error(), "username_case_conflict") {
		t.Fatalf("collision must stop the explicit migration: %v", err)
	}
	var table sql.NullString
	if err := db.QueryRow(`SELECT to_regclass('auth.login_failures')::text`).Scan(&table); err != nil || table.Valid {
		t.Fatalf("failed migration did not roll back DDL: %v %v", table, err)
	}
	if err := s.CheckSchema(ctx); err == nil || !strings.Contains(err.Error(), "-migrate") {
		t.Fatalf("HTTP startup must report explicit recovery: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM auth.users`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("migration changed conflicting accounts: %d %v", count, err)
	}
}
