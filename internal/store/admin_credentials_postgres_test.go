package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MoeclubM/metafusion-auth/internal/testutil"
)

func TestAdminCredentialInventoryAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	dsn := testutil.DSN(t)
	db := testutil.Database(t)
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	testutil.ResetAccounts(t, db)
	t.Cleanup(func() { testutil.ResetAccounts(t, db) })
	s.Tokens = newTestIssuer(t)
	admin, err := s.CreateUser(ctx, "inventory-admin", "", "inventory-password", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.CreateUser(ctx, "inventory-owner", "", "inventory-password", false, &admin)
	if err != nil {
		t.Fatal(err)
	}
	owner.Permissions = []string{"catalog.entity.edit", "catalog.relation.edit"}
	if err = s.SetUserGroups(ctx, owner.ID, []string{"catalog_editor"}, &admin); err != nil {
		t.Fatal(err)
	}
	pat, plain, err := s.CreatePersonalAccessToken(ctx, &owner, "first", []string{"catalog.entity.edit"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mcp, _, err := s.CreateMCPToken(ctx, &owner, "connection", []string{"catalog.entity.edit"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.ListAdminCredentials(ctx, AdminRecordFilter{OwnerID: owner.ID, Limit: 1}, &admin)
	if err != nil || page.Total != 2 || len(page.Items) != 1 || !page.HasMore {
		t.Fatalf("pagination: %+v %v", page, err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), plain) || strings.Contains(string(raw), "token_hash") {
		t.Fatal("credential material escaped projection")
	}
	if _, err = s.ListAdminCredentials(ctx, AdminRecordFilter{}, &owner); err == nil {
		t.Fatal("member enumerated credentials")
	}
	name := "renamed"
	if err = s.UpdateAdminCredential(ctx, &admin, pat.ID, AccessTokenUpdate{Name: &name}); err != nil {
		t.Fatal(err)
	}
	narrow := []string{}
	if err = s.UpdateAdminCredential(ctx, &admin, mcp.ID, AccessTokenUpdate{Scopes: &narrow}); err != nil {
		t.Fatal(err)
	}
	expand := []string{"catalog.entity.edit"}
	if err = s.UpdateAdminCredential(ctx, &admin, mcp.ID, AccessTokenUpdate{Scopes: &expand}); err == nil || err.Error() != "scope_expansion_requires_authorization" {
		t.Fatalf("MCP scope expansion: %v", err)
	}
	if err = s.RevokeAdminCredential(ctx, &admin, pat.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.IntrospectPersonalAccessToken(ctx, plain); err != ErrInvalidPAT {
		t.Fatalf("revoked PAT remained usable: %v", err)
	}
	page, err = s.ListAdminCredentials(ctx, AdminRecordFilter{OwnerID: owner.ID, Status: "revoked"}, &admin)
	if err != nil || page.Total != 1 || page.Items[0].Name != "renamed" {
		t.Fatalf("revoked filter: %+v %v", page, err)
	}
	if err = s.DeleteAdminCredential(ctx, &admin, mcp.ID); err != nil {
		t.Fatal(err)
	}
	page, err = s.ListAdminCredentials(ctx, AdminRecordFilter{OwnerID: owner.ID}, &admin)
	if err != nil || page.Total != 1 {
		t.Fatalf("deleted credential visible: %+v %v", page, err)
	}
}

func TestAdminOAuthInventoryAndRefreshAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	dsn := testutil.DSN(t)
	db := testutil.Database(t)
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	testutil.ResetAccounts(t, db)
	t.Cleanup(func() { testutil.ResetAccounts(t, db) })
	s.Tokens = newTestIssuer(t)
	admin, err := s.CreateUser(ctx, "oauth-inventory-admin", "", "inventory-password", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.CreateUser(ctx, "oauth-inventory-owner", "", "inventory-password", false, &admin)
	if err != nil {
		t.Fatal(err)
	}
	client, secret, err := s.CreateOAuthClient(ctx, OAuthClientInput{ID: "inventory-client", Name: strPtr("Inventory client"), RedirectURIs: &[]string{"https://inventory.example/callback"}, Scopes: &[]string{"openid", "profile"}}, &admin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM auth.oauth_clients WHERE id=$1", client.ID) })
	issue := func() OAuthGrant {
		t.Helper()
		if err = s.RecordOAuthAudit(ctx, OAuthAuditEntry{ActorID: owner.ID, SubjectID: owner.ID, ClientID: client.ID, Action: OAuthActionConsentAllow, Scopes: []string{"profile"}}); err != nil {
			t.Fatal(err)
		}
		code, _, err := s.CreateOAuthCode(ctx, client.ID, owner.ID, "https://inventory.example/callback", "profile", "", "")
		if err != nil {
			t.Fatal(err)
		}
		grant, err := s.ExchangeOAuthCode(ctx, client.ID, secret, code, "https://inventory.example/callback", "")
		if err != nil {
			t.Fatal(err)
		}
		return grant
	}
	grant := issue()
	if _, _, err = s.Refresh(ctx, grant.Token); err == nil || err.Error() != "invalid_token" {
		t.Fatalf("OAuth upgraded to session: %v", err)
	}
	page, err := s.ListAdminOAuthTokens(ctx, AdminRecordFilter{OwnerID: owner.ID, ClientID: client.ID, Limit: 1}, &admin)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || !page.Items[0].Active {
		t.Fatalf("token inventory %+v %v", page, err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), grant.Token) || strings.Contains(string(raw), "token_hash") {
		t.Fatal("OAuth secret/hash escaped projection")
	}
	grants, err := s.ListAdminOAuthGrants(ctx, AdminRecordFilter{OwnerID: owner.ID, ClientID: client.ID}, &admin)
	if err != nil || grants.Total != 1 || grants.Items[0].TokenCount != 1 {
		t.Fatalf("grant inventory %+v %v", grants, err)
	}
	self, err := s.ListOAuthGrants(ctx, owner.ID)
	if err != nil || len(self) != 1 || self[0].Active != grants.Items[0].Active || self[0].Name != grants.Items[0].ClientName {
		t.Fatalf("self/admin projection drift: %+v %v", self, err)
	}
	if err = s.DeleteAdminOAuthToken(ctx, &admin, page.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.OAuthUserinfo(ctx, grant.Token); err == nil {
		t.Fatal("deleted token still valid")
	}
	grant = issue()
	if n, err := s.RevokeAdminOAuthGrant(ctx, &admin, owner.ID, client.ID); err != nil || n != 1 {
		t.Fatalf("revoke grant n=%d err=%v", n, err)
	}
	grants, err = s.ListAdminOAuthGrants(ctx, AdminRecordFilter{OwnerID: owner.ID, Status: "inactive"}, &admin)
	if err != nil || grants.Total != 1 || grants.Items[0].Active {
		t.Fatalf("revoked history missing %+v %v", grants, err)
	}
	// A genuine first-party session still renews with current permissions.
	session, _, err := s.Login(ctx, "oauth-inventory-admin", "inventory-password")
	if err != nil {
		t.Fatal(err)
	}
	if next, user, err := s.Refresh(ctx, session); err != nil || next == "" || user.ID != admin.ID || !Can(&user, "auth.users.manage") {
		t.Fatalf("session refresh failed: %v", err)
	}
}
