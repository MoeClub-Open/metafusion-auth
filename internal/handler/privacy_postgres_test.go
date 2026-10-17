package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/store"
	"github.com/MoeclubM/metafusion-auth/internal/testutil"
	"github.com/google/uuid"
)

func TestPrivacyHTTPAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	r, st, recorder := newAuditTestServer(t)
	restore := saveSettings(t, st.DB, store.SettingRegistrationEnabled, store.SettingInviteRequired, store.SettingRegistrationGroups)
	t.Cleanup(restore)
	if err := st.UpdateSettings(ctx, map[string]any{store.SettingRegistrationEnabled: true, store.SettingInviteRequired: false, store.SettingRegistrationGroups: []string{"member"}}, settingsActor()); err != nil {
		t.Fatal(err)
	}
	ownerName := "privacy-http-" + uuid.NewString()[:8]
	w := doJSON(t, r, http.MethodPost, "/api/auth/register", "", `{"username":"`+ownerName+`","email":"`+ownerName+`@example.test","password":"`+chainTestPassword+`"}`)
	decodeIdentity := func(w *httptest.ResponseRecorder, wantFavorites, wantEmail bool) (string, store.User) {
		t.Helper()
		if w.Code != http.StatusOK {
			t.Fatalf("identity response = %d: %s", w.Code, w.Body.String())
		}
		var response struct {
			AccessToken string     `json:"access_token"`
			User        store.User `json:"user"`
		}
		decodeInto(t, w, &response)
		u := response.User
		if response.AccessToken == "" || u.ID == "" || u.FavoritesPublic == nil || *u.FavoritesPublic != wantFavorites || u.EmailPublic == nil || *u.EmailPublic != wantEmail {
			t.Fatalf("identity response must include actual privacy flags: %s", w.Body.String())
		}
		return response.AccessToken, u
	}
	ownerBearer, owner := decodeIdentity(w, true, false)
	ownerID := owner.ID
	t.Cleanup(func() { testutil.DeleteUser(t, st.DB, ownerID) })
	otherID, _, otherBearer := insertChainUser(t, ctx, st, "user")
	oauth, _, _, err := st.Tokens.SignOAuth(owner, "privacy-profile-client", []string{"openid", "profile"})
	if err != nil {
		t.Fatal(err)
	}
	idToken, _, err := st.Tokens.SignForAudience(owner, "privacy-profile-client")
	if err != nil {
		t.Fatal(err)
	}
	_, pat, err := st.CreatePersonalAccessToken(ctx, &owner, "privacy-profile-test", []string{"community.post.create"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertProfile := func(bearer, wantEmail string, wantFavorites, wantPublicEmail bool) {
		t.Helper()
		w := doJSON(t, r, http.MethodGet, "/api/users/"+ownerID, bearer, "")
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("public profile must be successful and uncacheable: %d %s", w.Code, w.Body.String())
		}
		var profile store.PublicProfile
		decodeInto(t, w, &profile)
		if profile.User.Email != wantEmail || profile.User.FavoritesPublic != wantFavorites || profile.User.EmailPublic != wantPublicEmail {
			t.Fatalf("public profile ignores privacy or token purpose: %s", w.Body.String())
		}
	}
	// subject 与本人相同也不能越过 OAuth scope 或把机器凭据当成站内会话。
	for _, bearer := range []string{"", otherBearer, oauth, idToken, pat} {
		assertProfile(bearer, "", true, false)
	}
	assertProfile(ownerBearer, ownerName+"@example.test", true, false)
	requestID := "mf-privacy-test-" + uuid.NewString()
	t.Cleanup(func() {
		if _, err := st.DB.ExecContext(ctx, `DELETE FROM audit.audit_log WHERE request_id=$1`, requestID); err != nil {
			t.Errorf("cleanup privacy audit: %v", err)
		}
	})
	readSettings := func(bearer string, wantFavorites, wantEmail bool) {
		t.Helper()
		w := doJSON(t, r, http.MethodGet, "/api/auth/privacy", bearer, "")
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("GET privacy = %d: %s", w.Code, w.Body.String())
		}
		var fields map[string]json.RawMessage
		decodeInto(t, w, &fields)
		if len(fields) != 2 {
			t.Fatalf("privacy response must contain two explicit booleans: %s", w.Body.String())
		}
		var settings store.PrivacySettings
		decodeInto(t, w, &settings)
		if settings.FavoritesPublic != wantFavorites || settings.EmailPublic != wantEmail || string(fields["favorites_public"]) != map[bool]string{true: "true", false: "false"}[wantFavorites] || string(fields["email_public"]) != map[bool]string{true: "true", false: "false"}[wantEmail] {
			t.Fatalf("unexpected privacy settings: %s", w.Body.String())
		}
	}
	readSettings(ownerBearer, true, false)
	w = auditRequest(t, r, http.MethodPatch, "/api/auth/privacy", ownerBearer, `{"favorites_public":false,"email_public":true}`, requestID)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("PATCH privacy = %d: %s", w.Code, w.Body.String())
	}
	var updated store.PrivacySettings
	decodeInto(t, w, &updated)
	if updated.FavoritesPublic || !updated.EmailPublic {
		t.Fatalf("PATCH did not return saved settings: %s", w.Body.String())
	}
	readSettings(ownerBearer, false, true)
	readSettings(otherBearer, true, false)

	// 继续使用更新前签发的 session：/me 必须回表，不能依赖 JWT 中过时投影。
	w = doJSON(t, r, http.MethodGet, "/api/auth/me", ownerBearer, "")
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET me = %d: %s", w.Code, w.Body.String())
	}
	var me struct {
		ID              string `json:"id"`
		FavoritesPublic *bool  `json:"favorites_public"`
		EmailPublic     *bool  `json:"email_public"`
	}
	decodeInto(t, w, &me)
	if me.ID != ownerID || me.FavoritesPublic == nil || *me.FavoritesPublic || me.EmailPublic == nil || !*me.EmailPublic {
		t.Fatalf("me did not read through current privacy: %s", w.Body.String())
	}
	// 新登录与续期的 user 响应也必须来自 DB，而不是注册时的旧标志。
	w = doJSON(t, r, http.MethodPost, "/api/auth/login", "", `{"username":"`+ownerName+`","password":"`+chainTestPassword+`"}`)
	decodeIdentity(w, false, true)
	w = doJSON(t, r, http.MethodPost, "/api/auth/refresh", ownerBearer, "")
	ownerBearer, _ = decodeIdentity(w, false, true)
	for _, bearer := range []string{"", ownerBearer, otherBearer, oauth, idToken, pat} {
		assertProfile(bearer, ownerName+"@example.test", false, true)
	}
	// 未授权请求和试图把 user_id 塞进自助 PATCH 都不能修改另一个账号。
	w = doJSON(t, r, http.MethodPatch, "/api/auth/privacy", "", `{"email_public":true}`)
	assertPrivacyError(t, w, http.StatusUnauthorized, "authentication_required")
	w = doJSON(t, r, http.MethodPatch, "/api/auth/privacy", ownerBearer, `{"user_id":"`+otherID+`","email_public":true}`)
	assertPrivacyError(t, w, http.StatusBadRequest, "invalid_payload")
	readSettings(otherBearer, true, false)
	readSettings(ownerBearer, false, true)

	if err := recorder.Flush(ctx); err != nil {
		t.Fatalf("flush privacy audit: %v", err)
	}
	rows := fetchAuditRows(t, st, requestID)
	if len(rows) != 1 {
		t.Fatalf("privacy change should emit one audit record, got %d", len(rows))
	}
	row := rows[0]
	if row.Action != "user.privacy_updated" || row.ActorUserID != ownerID || row.TargetType != "user" || row.TargetID != ownerID || row.Result != "success" || row.CredentialType != "session" || row.HTTPStatus != http.StatusOK {
		t.Fatalf("incorrect privacy audit record: %+v", row)
	}
	if strings.Contains(row.Changes, "@example.test") || strings.Contains(row.Changes, ownerBearer) {
		t.Fatalf("privacy audit leaked credentials or email: %s", row.Changes)
	}
}
