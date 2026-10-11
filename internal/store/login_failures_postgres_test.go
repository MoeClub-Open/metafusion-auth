package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MoeclubM/metafusion-auth/internal/testutil"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestLoginFailuresSharedAtomicAndPrivateAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	first := &Store{DB: db}
	if err := first.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })
	accounts := []string{"Concurrent-" + uuid.NewString(), "alias-" + uuid.NewString() + "@example.test"}
	ip := "ip-" + uuid.NewString()
	_, hashes := loginFailureKeys(accounts, ip)
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM auth.login_failures WHERE key_hash=ANY($1::text[])`, pq.Array(hashes)); err != nil {
			t.Error(err)
		}
	})
	const attempts = 24
	var wg sync.WaitGroup
	errors := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := first
			if i%2 != 0 {
				s = second
			}
			errors <- s.RecordLoginFailure(ctx, accounts, ip, 15*time.Minute)
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	// A newly constructed Store shares all committed counts: process restart does
	// not reset either account aliases or the IP spray budget.
	restarted := &Store{DB: db}
	buckets, err := restarted.LoginFailureBuckets(ctx, accounts, ip)
	if err != nil || len(buckets) != 3 {
		t.Fatalf("shared buckets: %+v, %v", buckets, err)
	}
	for _, bucket := range buckets {
		if bucket.Failures != attempts || bucket.RetryAfter <= 14*time.Minute {
			t.Fatalf("lost atomic increments or wrong window: %+v", bucket)
		}
	}
	var storedKeys []string
	rows, err := db.Query(`SELECT key_hash FROM auth.login_failures WHERE key_hash=ANY($1::text[])`, pq.Array(hashes))
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		storedKeys = append(storedKeys, key)
		if len(key) != 64 || key == accounts[0] || key == accounts[1] || key == ip {
			t.Fatalf("plain identifier persisted: %q", key)
		}
	}
	rows.Close()
	if len(storedKeys) != 3 {
		t.Fatal("missing stored keys")
	}
	if err := second.ClearLoginFailures(ctx, accounts); err != nil {
		t.Fatal(err)
	}
	buckets, err = first.LoginFailureBuckets(ctx, accounts, ip)
	if err != nil || len(buckets) != 1 || buckets[0].Dimension != "ip" || buckets[0].Failures != attempts {
		t.Fatalf("success must preserve IP failures: %+v %v", buckets, err)
	}
	// Expire using the DB clock; the next failed attempt starts at one, not 25.
	if _, err := db.Exec(`UPDATE auth.login_failures SET expires_at=now()-interval '1 second' WHERE key_hash=ANY($1::text[])`, pq.Array(hashes)); err != nil {
		t.Fatal(err)
	}
	if buckets, err := first.LoginFailureBuckets(ctx, accounts, ip); err != nil || len(buckets) != 0 {
		t.Fatalf("expired buckets still apply: %+v %v", buckets, err)
	}
	if err := second.RecordLoginFailure(ctx, accounts, ip, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	buckets, err = first.LoginFailureBuckets(ctx, accounts, ip)
	if err != nil || len(buckets) != 3 {
		t.Fatalf("renewed buckets: %+v %v", buckets, err)
	}
	for _, bucket := range buckets {
		if bucket.Failures != 1 {
			t.Fatalf("expired count was retained: %+v", bucket)
		}
	}
}
