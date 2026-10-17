package handler

import (
	"net/http"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/store"
)

func TestAdminCredentialRoutesEnforcePermissions(t *testing.T) {
	r, s := newTestServer(t)
	u := store.User{ID: "44444444-4444-4444-4444-444444444444", Username: "member", Permissions: []string{"catalog.entity.edit"}}
	token, _, _, err := s.Tokens.Sign(u)
	if err != nil {
		t.Fatal(err)
	}
	routes := [][2]string{
		{"GET", "/api/admin/credentials"}, {"PATCH", "/api/admin/credentials/invalid"}, {"POST", "/api/admin/credentials/invalid/revoke"}, {"DELETE", "/api/admin/credentials/invalid"},
		{"GET", "/api/admin/oauth/grants"}, {"POST", "/api/admin/oauth/grants/revoke"}, {"GET", "/api/admin/oauth/tokens"}, {"DELETE", "/api/admin/oauth/tokens/invalid"},
	}
	for _, route := range routes {
		for _, auth := range []string{"", token} {
			want := http.StatusUnauthorized
			if auth != "" {
				want = http.StatusForbidden
			}
			w := do(t, r, route[0], route[1], auth)
			if w.Code != want {
				t.Errorf("%v auth=%t status=%d body=%s", route, auth != "", w.Code, w.Body.String())
			}
		}
	}
}

func TestAdminCredentialFiltersRejectInvalidValues(t *testing.T) {
	r, s := newTestServer(t)
	u := store.User{ID: "44444444-4444-4444-4444-444444444444", Username: "manager", Permissions: []string{"auth.users.manage", "auth.oauth.manage"}}
	token, _, _, err := s.Tokens.Sign(u)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/admin/credentials?owner_id=invalid", "/api/admin/credentials?limit=0", "/api/admin/credentials?offset=-1", "/api/admin/credentials?kind=unknown", "/api/admin/credentials?status=unknown", "/api/admin/oauth/tokens?status=unknown", "/api/admin/oauth/grants?owner_id=invalid"} {
		w := do(t, r, http.MethodGet, path, token)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}
