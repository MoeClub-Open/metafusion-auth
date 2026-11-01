package store

import (
	"context"
	"testing"
	"time"

	"github.com/MoeclubM/metafusion-auth/internal/testutil"
	"github.com/google/uuid"
)

func TestPostgresLogCleanupScope(t *testing.T) {
	s := &Store{DB: testutil.Database(t)}
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service, other := "cleanup-"+uuid.NewString(), "cleanup-"+uuid.NewString()
	t.Cleanup(func() {
		s.DB.Exec("DELETE FROM audit.audit_log WHERE service IN ($1,$2)", service, other)
		s.DB.Exec("DELETE FROM audit.api_statistics WHERE service IN ($1,$2)", service, other)
	})
	before := time.Now().UTC().Truncate(24 * time.Hour)
	for _, target := range []string{service, other} {
		for _, at := range []time.Time{before.Add(-24 * time.Hour), before} {
			if _, err := s.DB.Exec(`INSERT INTO audit.audit_log(id,occurred_at,service,action) VALUES($1,$2,$3,'test')`, uuid.NewString(), at, target); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`INSERT INTO audit.api_statistics(day,service,method,route,status,requests,total_ms,max_ms) VALUES($1,$2,'GET','/api/test',200,5,10,4)`, at, target); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, kind := range []string{"audit", "statistics"} {
		count, err := s.PreviewLogCleanup(ctx, kind, before, service)
		if err != nil || count != 1 {
			t.Fatalf("preview %s: %d %v", kind, count, err)
		}
		removed, err := s.CleanupLogs(ctx, kind, before, service)
		if err != nil || removed != 1 {
			t.Fatalf("cleanup %s: %d %v", kind, removed, err)
		}
		count, err = s.PreviewLogCleanup(ctx, kind, before.Add(24*time.Hour), service)
		if err != nil || count != 1 {
			t.Fatalf("current day was removed: %s %d %v", kind, count, err)
		}
		count, err = s.PreviewLogCleanup(ctx, kind, before, other)
		if err != nil || count != 1 {
			t.Fatalf("other service changed: %s %d %v", kind, count, err)
		}
	}
	if _, err := s.CleanupLogs(ctx, "audit", before.Add(24*time.Hour), service); err == nil {
		t.Fatal("future cutoff accepted")
	}
}
