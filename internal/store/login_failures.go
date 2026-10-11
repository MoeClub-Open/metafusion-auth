package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

// LoginFailureBucket contains only the counts and remaining database-clock window.
// Plain account identifiers, IP addresses and passwords are never stored here.
type LoginFailureBucket struct {
	Dimension  string
	Failures   int
	RetryAfter time.Duration
}

func loginFailureKeys(accounts []string, ip string) (dimensions, hashes []string) {
	keys := map[string]string{}
	add := func(dimension, identifier string) {
		identifier = strings.TrimSpace(identifier)
		if identifier == "" {
			return
		}
		if dimension == "account" {
			identifier = strings.ToLower(identifier)
		}
		sum := sha256.Sum256([]byte(dimension + "\x00" + identifier))
		keys[dimension+":"+hex.EncodeToString(sum[:])] = dimension
	}
	for _, account := range accounts {
		add("account", account)
	}
	add("ip", ip)
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted) // consistent row locking order across all replicas
	for _, key := range sorted {
		dimension := keys[key]
		dimensions = append(dimensions, dimension)
		hashes = append(hashes, strings.TrimPrefix(key, dimension+":"))
	}
	return dimensions, hashes
}

func (s *Store) LoginFailureBuckets(ctx context.Context, accounts []string, ip string) ([]LoginFailureBucket, error) {
	if s == nil || s.DB == nil {
		return nil, sql.ErrConnDone
	}
	dimensions, hashes := loginFailureKeys(accounts, ip)
	if len(hashes) == 0 {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT f.dimension,f.failures,
		EXTRACT(EPOCH FROM (f.expires_at-statement_timestamp()))::double precision
		FROM auth.login_failures f
		JOIN unnest($1::text[],$2::text[]) k(dimension,key_hash)
		ON f.dimension=k.dimension AND f.key_hash=k.key_hash
		WHERE f.expires_at>statement_timestamp()`, pq.Array(dimensions), pq.Array(hashes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LoginFailureBucket{}
	for rows.Next() {
		var bucket LoginFailureBucket
		var seconds float64
		if err := rows.Scan(&bucket.Dimension, &bucket.Failures, &seconds); err != nil {
			return nil, err
		}
		bucket.RetryAfter = time.Duration(seconds * float64(time.Second))
		out = append(out, bucket)
	}
	return out, rows.Err()
}

func (s *Store) RecordLoginFailure(ctx context.Context, accounts []string, ip string, window time.Duration) error {
	if s == nil || s.DB == nil {
		return sql.ErrConnDone
	}
	if window < time.Millisecond {
		return fmt.Errorf("invalid login failure window")
	}
	dimensions, hashes := loginFailureKeys(accounts, ip)
	if len(hashes) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Every failed credential attempt atomically increments all its dimensions.
	// PostgreSQL owns the clock and ON CONFLICT lock; replicas cannot lose increments.
	if _, err := tx.ExecContext(ctx, `INSERT INTO auth.login_failures AS f(dimension,key_hash,failures,expires_at)
		SELECT dimension,key_hash,1,statement_timestamp()+$3*interval '1 millisecond'
		FROM unnest($1::text[],$2::text[]) k(dimension,key_hash) ORDER BY dimension,key_hash
		ON CONFLICT(dimension,key_hash) DO UPDATE SET
		failures=CASE WHEN f.expires_at<=statement_timestamp() THEN 1 ELSE LEAST(f.failures+1,1000000) END,
		expires_at=CASE WHEN f.expires_at<=statement_timestamp()
		THEN statement_timestamp()+$3*interval '1 millisecond' ELSE f.expires_at END`,
		pq.Array(dimensions), pq.Array(hashes), window.Milliseconds()); err != nil {
		return err
	}
	// Bounded expiry cleanup avoids an unbounded table from random account names.
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth.login_failures WHERE (dimension,key_hash) IN (
		SELECT dimension,key_hash FROM auth.login_failures WHERE expires_at<=statement_timestamp()
		ORDER BY expires_at LIMIT 128 FOR UPDATE SKIP LOCKED
	)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ClearLoginFailures(ctx context.Context, accounts []string) error {
	if s == nil || s.DB == nil {
		return sql.ErrConnDone
	}
	_, hashes := loginFailureKeys(accounts, "")
	if len(hashes) == 0 {
		return nil
	}
	// A successful login clears its account keys; the IP spray budget survives.
	_, err := s.DB.ExecContext(ctx, `DELETE FROM auth.login_failures
		WHERE dimension='account' AND key_hash=ANY($1::text[])`, pq.Array(hashes))
	return err
}
