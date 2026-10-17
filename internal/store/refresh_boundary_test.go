package store

import (
	"context"
	"testing"
)

// Rejected identities must never reach the account reload or session issuer.
// A nil database makes an accidental privilege reload fail this regression.
func TestRefreshRejectsThirdPartyBeforeAccountReload(t *testing.T) {
	issuer := newIssuer(t, "https://findverse.cc/api", "metafusion")
	admin := User{ID: "11111111-1111-1111-1111-111111111111", Username: "root", Permissions: []string{"*"}}
	oauth, _, _, err := issuer.SignOAuth(admin, "external-client", []string{"profile"})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := issuer.SignForAudience(admin, "external-client")
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{Tokens: issuer}
	for name, token := range map[string]string{"oauth": oauth, "id_token": id} {
		t.Run(name, func(t *testing.T) {
			next, user, err := s.Refresh(context.Background(), token)
			if err == nil || err.Error() != "invalid_token" || next != "" || user.ID != "" {
				t.Fatalf("third-party identity refreshed: next=%t user=%+v err=%v", next != "", user, err)
			}
		})
	}
}

func TestAdminCredentialMutationRejectsDelegatedManager(t *testing.T) {
	s := &Store{}
	actor := &User{ID: "11111111-1111-1111-1111-111111111111", TokenUse: "pat", Permissions: []string{"*"}}
	for name, call := range map[string]func() error{
		"edit": func() error {
			return s.UpdateAdminCredential(context.Background(), actor, actor.ID, AccessTokenUpdate{})
		},
		"revoke": func() error { return s.RevokeAdminCredential(context.Background(), actor, actor.ID) },
		"delete": func() error { return s.DeleteAdminCredential(context.Background(), actor, actor.ID) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || err.Error() != "forbidden" {
				t.Fatalf("delegated key managed credentials: %v", err)
			}
		})
	}
}
