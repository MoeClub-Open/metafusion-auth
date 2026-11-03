package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/MoeclubM/metafusion-auth/internal/store"
)

func newCredentialsStatusServer(t *testing.T, internalToken string, lookup func(context.Context, string) (store.CredentialsStatus, error)) (*gin.Engine, *store.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &store.Store{Tokens: newTestIssuer(t)}
	h := New(s)
	h.SetInternalToken(internalToken)
	h.credentialsStatusFn = lookup
	r := gin.New()
	h.Register(r)
	return r, s
}

func doCredentialsStatus(t *testing.T, r *gin.Engine, userID, bearer, internalToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/credentials-status/"+userID, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if internalToken != "" {
		req.Header.Set(InternalTokenHeader, internalToken)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCredentialsStatusRejectsAnonymous(t *testing.T) {
	called := false
	r, _ := newCredentialsStatusServer(t, "svc-secret", func(context.Context, string) (store.CredentialsStatus, error) {
		called = true
		return store.CredentialsStatus{}, nil
	})
	w := doCredentialsStatus(t, r, "11111111-1111-1111-1111-111111111111", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("匿名应 401，实际 %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error":"authentication_required"`) {
		t.Fatalf("匿名错误码应为 authentication_required，实际 %s", body)
	}
	for _, leak := range []string{"user_not_found", "token_version", "banned"} {
		if strings.Contains(body, leak) {
			t.Fatalf("匿名响应不得泄露 %s：%s", leak, body)
		}
	}
	if called {
		t.Fatal("匿名请求不得触达凭据快照查询")
	}
}

func TestCredentialsStatusSelfBearerOK(t *testing.T) {
	const uid = "22222222-2222-2222-2222-222222222222"
	r, s := newCredentialsStatusServer(t, "svc-secret", func(_ context.Context, id string) (store.CredentialsStatus, error) {
		if id != uid {
			t.Fatalf("lookup id=%s", id)
		}
		return store.CredentialsStatus{UserID: uid, TokenVersion: 4, Banned: false}, nil
	})
	token, _, _, err := s.Tokens.Sign(store.User{ID: uid, Username: "self", Groups: []string{"member"}})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	w := doCredentialsStatus(t, r, uid, token, "")
	if w.Code != http.StatusOK {
		t.Fatalf("本人应 200，实际 %d %s", w.Code, w.Body.String())
	}
	var st store.CredentialsStatus
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.UserID != uid || st.TokenVersion != 4 || st.Banned {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestCredentialsStatusCrossUserForbidden(t *testing.T) {
	const selfID = "33333333-3333-3333-3333-333333333333"
	const otherID = "44444444-4444-4444-4444-444444444444"
	called := false
	r, s := newCredentialsStatusServer(t, "svc-secret", func(context.Context, string) (store.CredentialsStatus, error) {
		called = true
		return store.CredentialsStatus{UserID: otherID, TokenVersion: 9, Banned: true}, nil
	})
	token, _, _, err := s.Tokens.Sign(store.User{ID: selfID, Username: "self", Groups: []string{"member"}})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	w := doCredentialsStatus(t, r, otherID, token, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("跨用户应 403，实际 %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error":"forbidden"`) {
		t.Fatalf("跨用户错误码应为 forbidden，实际 %s", body)
	}
	for _, leak := range []string{"user_not_found", "token_version", `"banned"`} {
		if strings.Contains(body, leak) {
			t.Fatalf("跨用户响应不得泄露 %s：%s", leak, body)
		}
	}
	if called {
		t.Fatal("跨用户不得触达凭据快照查询（防枚举）")
	}
}

func TestCredentialsStatusServiceTokenOK(t *testing.T) {
	const uid = "55555555-5555-5555-5555-555555555555"
	r, _ := newCredentialsStatusServer(t, "svc-secret", func(_ context.Context, id string) (store.CredentialsStatus, error) {
		return store.CredentialsStatus{UserID: id, TokenVersion: 2, Banned: true}, nil
	})
	w := doCredentialsStatus(t, r, uid, "", "svc-secret")
	if w.Code != http.StatusOK {
		t.Fatalf("服务身份应 200，实际 %d %s", w.Code, w.Body.String())
	}
	var st store.CredentialsStatus
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.UserID != uid || st.TokenVersion != 2 || !st.Banned {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestCredentialsStatusServiceTokenMissingUserNotFound(t *testing.T) {
	r, _ := newCredentialsStatusServer(t, "svc-secret", func(context.Context, string) (store.CredentialsStatus, error) {
		return store.CredentialsStatus{}, fmt.Errorf("user_not_found")
	})
	w := doCredentialsStatus(t, r, "66666666-6666-6666-6666-666666666666", "", "svc-secret")
	if w.Code != http.StatusNotFound {
		t.Fatalf("内部身份下缺失用户应 404，实际 %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"error":"user_not_found"`) {
		t.Fatalf("错误码应为 user_not_found，实际 %s", w.Body.String())
	}
}

func TestCredentialsStatusEmptyInternalTokenFailClosed(t *testing.T) {
	called := false
	r, _ := newCredentialsStatusServer(t, "", func(context.Context, string) (store.CredentialsStatus, error) {
		called = true
		return store.CredentialsStatus{UserID: "x", TokenVersion: 1}, nil
	})
	// 即使带了头，空配置也不得放行服务身份。
	w := doCredentialsStatus(t, r, "77777777-7777-7777-7777-777777777777", "", "anything")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("空 INTERNAL_API_TOKEN 应 fail-closed 401，实际 %d %s", w.Code, w.Body.String())
	}
	if called {
		t.Fatal("空配置下错误服务头不得触达查询")
	}
}

func TestCredentialsStatusWrongInternalTokenRejected(t *testing.T) {
	r, _ := newCredentialsStatusServer(t, "svc-secret", func(context.Context, string) (store.CredentialsStatus, error) {
		return store.CredentialsStatus{UserID: "x", TokenVersion: 1}, nil
	})
	w := doCredentialsStatus(t, r, "88888888-8888-8888-8888-888888888888", "", "wrong-secret")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("错误服务令牌应 401，实际 %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "token_version") {
		t.Fatalf("错误服务令牌不得泄露快照：%s", w.Body.String())
	}
}
