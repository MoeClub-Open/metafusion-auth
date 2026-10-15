package store

import (
	"context"
	"strings"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/testutil"
)

func TestManagedCredentialsAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	s := &Store{DB: testutil.Database(t)}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner := patTestUser(t, ctx, s, "catalog_editor")
	other := patTestUser(t, ctx, s, "catalog_editor")
	var personal PersonalAccessToken
	var personalSecret string
	for i := 0; i < MaxPersonalAccessTokensPerUser; i++ {
		item, secret, err := s.CreatePersonalAccessToken(ctx, owner, "Personal", []string{"catalog.entity.edit"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		personal, personalSecret = item, secret
	}
	delegated, secret, err := s.CreateMCPToken(ctx, owner, "Client", []string{"catalog.entity.edit", "catalog.relation.edit"})
	if err != nil {
		t.Fatalf("MCP must work even at the personal key limit: %v", err)
	}
	items, err := s.ListPersonalAccessTokens(ctx, owner.ID)
	if err != nil || len(items) != MaxPersonalAccessTokensPerUser {
		t.Fatalf("MCP leaked into personal list: %d %v", len(items), err)
	}
	if err := s.DeletePersonalAccessToken(ctx, owner.ID, delegated.ID); err == nil {
		t.Fatal("personal endpoint removed MCP credential")
	}
	if err := s.RevokeMCPToken(ctx, owner.ID, personal.ID); err == nil {
		t.Fatal("MCP endpoint revoked personal key")
	}
	if err := s.UpdateMCPToken(ctx, owner, delegated.ID, []string{"catalog.entity.edit"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMCPToken(ctx, owner, delegated.ID, []string{"catalog.entity.edit", "catalog.relation.edit"}); err == nil || err.Error() != "scope_expansion_requires_authorization" {
		t.Fatalf("MCP scope expansion accepted: %v", err)
	}
	if n := patTotalCount(t, ctx, s, owner.ID); n != MaxPersonalAccessTokensPerUser+1 {
		t.Fatalf("narrowing minted credentials: %d", n)
	}
	current, err := s.IntrospectPersonalAccessToken(ctx, secret)
	if err != nil || len(current.Scopes) != 2 {
		t.Fatalf("original credential no longer usable after narrowing: %v", err)
	}
	name := "Renamed"
	if proofOwner, proofID, err := s.RevokeMCPTokenByValue(ctx, personalSecret); err != nil || proofOwner != "" || proofID != "" {
		t.Fatalf("a personal secret must not be accepted by delegated revocation: %v", err)
	}
	if _, err := s.IntrospectPersonalAccessToken(ctx, personalSecret); err != nil {
		t.Fatalf("delegated revocation affected a personal key: %v", err)
	}
	for _, actor := range []*User{other, owner} {
		err := s.UpdatePersonalAccessToken(ctx, actor, personal.ID, AccessTokenUpdate{Name: &name})
		if (actor == owner) != (err == nil) {
			t.Fatalf("ownership boundary: %v", err)
		}
	}
	changed := []string{"catalog.relation.edit"}
	if err := s.UpdatePersonalAccessToken(ctx, owner, personal.ID, AccessTokenUpdate{Scopes: &changed}); err != nil {
		t.Fatal(err)
	}
	current, err = s.IntrospectPersonalAccessToken(ctx, personalSecret)
	if err != nil || current.TokenName != name || len(current.Scopes) != 1 || current.Scopes[0] != changed[0] {
		t.Fatalf("edits did not retain the original secret: %v", err)
	}
	if err := s.DeletePersonalAccessToken(ctx, other.ID, personal.ID); err == nil {
		t.Fatal("another user deleted the key")
	}
	for i := 0; i < 2; i++ {
		if err := s.DeletePersonalAccessToken(ctx, owner.ID, personal.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.IntrospectPersonalAccessToken(ctx, personalSecret); err != ErrInvalidPAT {
		t.Fatalf("deleted token still valid: %v", err)
	}
	items, err = s.ListPersonalAccessTokens(ctx, owner.ID)
	if err != nil || len(items) != MaxPersonalAccessTokensPerUser-1 {
		t.Fatalf("deleted key still listed: %d %v", len(items), err)
	}
	if _, _, err := s.CreatePersonalAccessToken(ctx, owner, "Replacement", []string{"catalog.entity.edit"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreatePersonalAccessToken(ctx, owner, "Invalid", []string{"mcp.connect"}, 0); err == nil {
		t.Fatal("reserved MCP scope issued as personal key")
	}
	for i := 0; i < 2; i++ {
		proofOwner, proofID, err := s.RevokeMCPTokenByValue(ctx, secret)
		if err != nil || proofOwner != owner.ID || proofID != delegated.ID {
			t.Fatalf("delegation proof revocation failed: %v", err)
		}
	}
	if err := s.RevokeMCPToken(ctx, owner.ID, delegated.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IntrospectPersonalAccessToken(ctx, secret); err != ErrInvalidPAT {
		t.Fatalf("revoked MCP credential accepted: %v", err)
	}
}

func TestMCPClassificationMigrationPreservesCredentials(t *testing.T) {
	ctx := context.Background()
	s := &Store{DB: testutil.Database(t)}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner := patTestUser(t, ctx, s, "catalog_editor")
	item, secret, err := s.CreateMCPToken(ctx, owner, "Legacy connection", []string{"catalog.entity.edit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, "UPDATE auth.personal_access_tokens SET purpose='personal' WHERE id=$1", item.ID); err != nil {
		t.Fatal(err)
	}
	// A name alone must never cause a personal key to be classified as MCP.
	personal, _, err := s.CreatePersonalAccessToken(ctx, owner, "MCP: personal label", []string{"catalog.entity.edit"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.ListPersonalAccessTokens(ctx, owner.ID)
	if err != nil || len(items) != 1 || items[0].ID != personal.ID {
		t.Fatalf("migration misclassified credentials: %v %v", items, err)
	}
	if p, err := s.IntrospectPersonalAccessToken(ctx, secret); err != nil || !strings.Contains(strings.Join(p.Scopes, " "), "mcp.connect") {
		t.Fatalf("migration invalidated existing connection: %v", err)
	}
}
