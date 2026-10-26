package store

import (
	"context"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/testutil"
)

// 改密 / 全量登出 / 管理员重置 / 封禁后，旧会话 JWT 必须因 token_version 不匹配而立即失效。
// 这是 H1/H2 的共享修复：批量吊销靠 bump 世代，而不是扫 jti。
func TestCredentialsVersionRevokesSessionJWTAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	dsn := testutil.DSN(t)
	db := testutil.Database(t)
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Init(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	testutil.ResetAccounts(t, db)
	t.Cleanup(func() { testutil.ResetAccounts(t, db) })
	s.Tokens = newTestIssuer(t)

	const pwd = "version-test-secret-1"
	admin, err := s.CreateUser(ctx, "tv-admin", "", pwd, true, nil)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	member, err := s.CreateUser(ctx, "tv-member", "", pwd, false, &admin)
	if err != nil {
		t.Fatalf("member: %v", err)
	}

	assertRevoked := func(t *testing.T, label, token string) {
		t.Helper()
		if _, err := s.Authenticate(ctx, token); err == nil {
			t.Fatalf("%s 后旧会话 JWT 仍可用", label)
		}
		st, err := s.CredentialsStatus(ctx, member.ID)
		if err != nil {
			t.Fatalf("credentials status: %v", err)
		}
		if st.TokenVersion < 1 {
			t.Fatalf("%s 后 token_version 应至少为 1，实际 %d", label, st.TokenVersion)
		}
	}

	// ChangePassword
	token, _, err := s.Login(ctx, member.Username, pwd)
	if err != nil {
		t.Fatalf("login before change: %v", err)
	}
	claims, err := s.Tokens.Verify(token)
	if err != nil || claims.TokenVersion != 0 {
		t.Fatalf("签发时 tv 应为 0: claims=%+v err=%v", claims, err)
	}
	if err := s.ChangePassword(ctx, member.ID, pwd, "version-test-secret-2"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	assertRevoked(t, "ChangePassword", token)

	// LogoutAll
	token, _, err = s.Login(ctx, member.Username, "version-test-secret-2")
	if err != nil {
		t.Fatalf("login before logout-all: %v", err)
	}
	if err := s.LogoutAll(ctx, member.ID); err != nil {
		t.Fatalf("logout all: %v", err)
	}
	assertRevoked(t, "LogoutAll", token)

	// ResetUserPassword
	token, _, err = s.Login(ctx, member.Username, "version-test-secret-2")
	if err != nil {
		t.Fatalf("login before reset: %v", err)
	}
	if err := s.ResetUserPassword(ctx, member.ID, "version-test-secret-3", &admin); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	assertRevoked(t, "ResetUserPassword", token)

	// SetUserBanned
	token, _, err = s.Login(ctx, member.Username, "version-test-secret-3")
	if err != nil {
		t.Fatalf("login before ban: %v", err)
	}
	if _, err := s.SetUserBanned(ctx, member.ID, true, &admin); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if _, err := s.Authenticate(ctx, token); err == nil || err.Error() != "account_banned" {
		// 封禁路径优先返回 account_banned；世代也已 bump。
		if err == nil {
			t.Fatal("封禁后旧会话 JWT 仍可用")
		}
		if err.Error() != "account_banned" && err.Error() != "invalid_token" {
			t.Fatalf("封禁后应 account_banned 或 invalid_token，实际 %v", err)
		}
	}
	st, err := s.CredentialsStatus(ctx, member.ID)
	if err != nil || !st.Banned || st.TokenVersion < 1 {
		t.Fatalf("封禁后 status 不符: %+v err=%v", st, err)
	}

	// 单会话 Logout 不 bump（只作废当前 jti）。
	if _, err := s.SetUserBanned(ctx, member.ID, false, &admin); err != nil {
		t.Fatalf("unban: %v", err)
	}
	before, err := s.CredentialsStatus(ctx, member.ID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	token, _, err = s.Login(ctx, member.Username, "version-test-secret-3")
	if err != nil {
		t.Fatalf("login before single logout: %v", err)
	}
	if err := s.Logout(ctx, token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	after, err := s.CredentialsStatus(ctx, member.ID)
	if err != nil {
		t.Fatalf("status after logout: %v", err)
	}
	if after.TokenVersion != before.TokenVersion {
		t.Fatalf("单会话登出不应 bump token_version: before=%d after=%d", before.TokenVersion, after.TokenVersion)
	}
	if _, err := s.Authenticate(ctx, token); err == nil {
		t.Fatal("单会话登出后当前 JWT 应因 jti 吊销而失效")
	}
}
