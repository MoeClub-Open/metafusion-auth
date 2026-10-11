package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MoeclubM/metafusion-auth/internal/store"
)

type failingLoginStorage struct {
	readErr, writeErr, clearErr error
	buckets                     []store.LoginFailureBucket
}

func (f *failingLoginStorage) LoginFailureBuckets(context.Context, []string, string) ([]store.LoginFailureBucket, error) {
	return f.buckets, f.readErr
}
func (f *failingLoginStorage) RecordLoginFailure(context.Context, []string, string, time.Duration) error {
	return f.writeErr
}
func (f *failingLoginStorage) ClearLoginFailures(context.Context, []string) error {
	return f.clearErr
}

func TestLoginGuardStorageFailureNeverFallsBackToMemory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend failingLoginStorage
	}{
		{"read", failingLoginStorage{readErr: errors.New("database offline")}},
		{"failure write", failingLoginStorage{writeErr: errors.New("database offline")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			r, guard := newLoginGuardTestServer(t, &now)
			guard.persistence = &tc.backend
			w := loginAttempt(t, r, "failure-"+tc.name)
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "login_guard_unavailable") {
				t.Fatalf("storage fault must return 503: %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "access_token") {
				t.Fatal("failed guard returned login credentials")
			}
			assertNoSessionCookie(t, w)
			if len(guard.accounts)+len(guard.ips) != 0 {
				t.Fatal("storage failure fell back to local counters")
			}
		})
	}
}

func TestLoginGuardRespectsSharedAccountAndIPWindows(t *testing.T) {
	for _, tc := range []struct {
		dimension string
		count     int
	}{
		{"account", loginGuardAccountMaxFailure},
		{"ip", loginGuardIPMaxFailure},
	} {
		t.Run(tc.dimension, func(t *testing.T) {
			now := time.Now()
			r, guard := newLoginGuardTestServer(t, &now)
			guard.persistence = &failingLoginStorage{buckets: []store.LoginFailureBucket{{Dimension: tc.dimension, Failures: tc.count, RetryAfter: 93 * time.Second}}}
			w := loginAttempt(t, r, "shared-window")
			if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "93" || !strings.Contains(w.Body.String(), "login_blocked") {
				t.Fatalf("shared block ignored: %d %s %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
			}
		})
	}
}
