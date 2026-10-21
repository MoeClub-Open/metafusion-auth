package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/MoeclubM/metafusion-auth/internal/store"
	"github.com/MoeclubM/metafusion-auth/internal/testutil"
)

func TestLoginGuardHTTPSharedAndResetAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	first := &store.Store{DB: db}
	if err := first.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := store.Open(ctx, testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })
	const ip = "198.51.100.201"
	missing := "missing-" + uuid.NewString()
	username := "guard-" + uuid.NewString()
	uid := uuid.NewString()
	const password = "guard-test-valid-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auth.users(id,username,password_hash) VALUES($1,$2,$3)`, uid, username, string(hash)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testutil.DeleteUser(t, db, uid)
		if err := first.ClearLoginFailures(ctx, []string{missing, username}); err != nil {
			t.Error(err)
		}
		sum := sha256.Sum256([]byte("ip\x00" + ip))
		if _, err := db.Exec(`DELETE FROM auth.login_failures WHERE dimension='ip' AND key_hash=$1`, hex.EncodeToString(sum[:])); err != nil {
			t.Error(err)
		}
	})
	engine := func(s *store.Store, backend loginFailureStore) *gin.Engine {
		h := New(s)
		h.audit = nil
		h.rateLimitPolicy = func(context.Context) (bool, int) { return false, 180 }
		if backend != nil {
			h.loginGuardFactory = func() *loginGuard {
				guard := newLoginGuard()
				guard.persistence = backend
				return guard
			}
		}
		r := gin.New()
		h.Register(r)
		return r
	}
	r1, r2 := engine(first, nil), engine(second, nil)
	login := func(r *gin.Engine, name, pass string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, name, pass)))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = ip + ":1234"
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		return response
	}
	for i := 0; i < 5; i++ {
		r := r1
		if i%2 != 0 {
			r = r2
		}
		if w := login(r, missing, "wrong"); w.Code != http.StatusUnauthorized {
			t.Fatalf("failed attempt %d: %d %s", i+1, w.Code, w.Body.String())
		}
	}
	// A newly constructed HTTP engine shares prior replicas' failures.
	if w := login(engine(second, nil), missing, "wrong"); w.Code != http.StatusUnauthorized {
		t.Fatalf("new replica hard-locked account: %d %s", w.Code, w.Body.String())
	}
	for i := 0; i < 5; i++ {
		if w := login(r1, username, "wrong"); w.Code != http.StatusUnauthorized {
			t.Fatalf("existing account wrong password: %d", w.Code)
		}
	}
	if w := login(r2, username, password); w.Code != http.StatusOK {
		t.Fatalf("successful login failed: %d %s", w.Code, w.Body.String())
	}
	buckets, err := first.LoginFailureBuckets(ctx, []string{username}, ip)
	if err != nil || len(buckets) != 1 || buckets[0].Dimension != "ip" || buckets[0].Failures != 11 {
		t.Fatalf("HTTP success did not reset only account failures: %+v %v", buckets, err)
	}
	// The attacker's source is blocked across replicas, while another source can
	// still authenticate with the correct password despite the shared account count.
	for i := 11; i < loginGuardIPMaxFailure; i++ {
		if err := first.RecordLoginFailure(ctx, []string{username}, ip, 15*time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if w := login(engine(second, nil), username, password); w.Code != http.StatusTooManyRequests {
		t.Fatalf("source budget not shared: %d %s", w.Code, w.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "198.51.100.202:1234"
	victim := httptest.NewRecorder()
	r2.ServeHTTP(victim, request)
	if victim.Code != http.StatusOK || victim.Header().Get("Set-Cookie") == "" {
		t.Fatalf("another source was locked: %d %s", victim.Code, victim.Body.String())
	}
	// Clearing a successful login must finish before the token/cookie is sent.
	broken := engine(first, &failingLoginStorage{clearErr: errors.New("database offline")})
	w := login(broken, username, password)
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "access_token") || !strings.Contains(w.Body.String(), "login_guard_unavailable") {
		t.Fatalf("clear fault leaked successful credentials: %d %s", w.Code, w.Body.String())
	}
	assertNoSessionCookie(t, w)
}
