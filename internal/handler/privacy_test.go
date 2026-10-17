package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/store"
)

func assertPrivacyError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
	}
	decodeInto(t, w, &payload)
	if w.Code != status || payload.Error != code {
		t.Fatalf("privacy error = %d %s, want %d %s", w.Code, w.Body.String(), status, code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("privacy errors must not be cached: %q", w.Header().Get("Cache-Control"))
	}
}

func TestPrivacyRequiresFirstPartySession(t *testing.T) {
	fake := &fakePATs{plain: testPATPlain, principal: &store.PATPrincipal{UserID: "77777777-7777-7777-7777-777777777777", Permissions: []string{"catalog.entity.edit"}}}
	r, s := newPATTestServer(t, fake)
	user := store.User{ID: fake.principal.UserID, Username: "privacy-owner", Permissions: []string{"*"}}
	session := signBearer(t, s, user)
	oauth, _, _, err := s.Tokens.SignOAuth(user, "privacy-test-client", []string{"openid", "profile", "email"})
	if err != nil {
		t.Fatal(err)
	}
	idToken, _, err := s.Tokens.SignForAudience(user, "privacy-test-client")
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range []struct{ name, token string }{{"anonymous", ""}, {"pat", testPATPlain}, {"oauth", oauth}, {"id-token", idToken}} {
		for _, method := range []string{http.MethodGet, http.MethodPatch} {
			t.Run(credential.name+"/"+method, func(t *testing.T) {
				w := doJSON(t, r, method, "/api/auth/privacy", credential.token, `{"email_public":true}`)
				assertPrivacyError(t, w, http.StatusUnauthorized, "authentication_required")
				assertNoSessionCookie(t, w)
			})
			if credential.token == "" {
				continue
			}
			t.Run(credential.name+"/cookie-fallback/"+method, func(t *testing.T) {
				req := httptest.NewRequest(method, "/api/auth/privacy", strings.NewReader(`{"email_public":true}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+credential.token)
				req.AddCookie(&http.Cookie{Name: "mf_session", Value: session})
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				assertPrivacyError(t, w, http.StatusUnauthorized, "authentication_required")
			})
		}
	}
}

func TestPrivacyPatchRejectsAmbiguousPayloads(t *testing.T) {
	r, s := newPATTestServer(t, &fakePATs{})
	bearer := signBearer(t, s, store.User{ID: "77777777-7777-7777-7777-777777777777", Username: "privacy-owner"})
	for _, payload := range []string{
		``, `{}`, `null`, `[]`, `true`,
		`{"favorites_public":null}`, `{"email_public":null}`,
		`{"favorites_public":"false"}`, `{"email_public":0}`,
		`{"favorites_public":[]}`, `{"email_public":{}}`,
		`{"email_public":true,"favorites_public":null}`,
		`{"user_id":"someone-else","email_public":true}`,
		`{"unknown":true}`, `{"email_public":true} {"favorites_public":false}`,
	} {
		t.Run(payload, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPatch, "/api/auth/privacy", bearer, payload)
			assertPrivacyError(t, w, http.StatusBadRequest, "invalid_payload")
		})
	}
}
