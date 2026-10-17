package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/testutil"
	"github.com/google/uuid"
)

func privacyBool(value bool) *bool { return &value }

// 隐私不是浏览器本地偏好：公开投影与第一方 /me 必须读同一组持久化设置。
func TestPrivacySettingsAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	s := &Store{DB: db}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id, otherID := uuid.NewString(), uuid.NewString()
	for i, userID := range []string{id, otherID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO auth.users(id,username,email,password_hash) VALUES($1,$2,$3,'unused')`, userID, "privacy-"+userID[:8], []string{"privacy-owner@example.test", "privacy-other@example.test"}[i]); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { testutil.DeleteUser(t, db, userID) })
	}
	assertSettings := func(wantFavorites, wantEmail bool) {
		t.Helper()
		settings, err := s.Privacy(ctx, id)
		if err != nil || settings.FavoritesPublic != wantFavorites || settings.EmailPublic != wantEmail {
			t.Fatalf("privacy = %+v, err = %v; want favorites=%v email=%v", settings, err, wantFavorites, wantEmail)
		}
	}
	assertProfile := func(viewer string, wantEmail string, wantFavorites, wantPublicEmail bool) {
		t.Helper()
		profile, err := s.PublicProfile(ctx, id, viewer)
		if err != nil {
			t.Fatal(err)
		}
		if profile.User.Email != wantEmail || profile.User.FavoritesPublic != wantFavorites || profile.User.EmailPublic != wantPublicEmail {
			t.Fatalf("viewer %q: unexpected public projection: %+v", viewer, profile.User)
		}
	}

	// 默认继续公开收藏，邮箱默认不公开；本人仍可以取得自己的邮箱。
	assertSettings(true, false)
	assertProfile("", "", true, false)
	assertProfile(otherID, "", true, false)
	assertProfile(id, "privacy-owner@example.test", true, false)

	if settings, err := s.UpdatePrivacy(ctx, id, PrivacyPatch{FavoritesPublic: privacyBool(false)}); err != nil || settings.FavoritesPublic || settings.EmailPublic {
		t.Fatalf("partial favorites update = %+v, err = %v", settings, err)
	}
	if settings, err := s.UpdatePrivacy(ctx, id, PrivacyPatch{EmailPublic: privacyBool(true)}); err != nil || settings.FavoritesPublic || !settings.EmailPublic {
		t.Fatalf("partial email update = %+v, err = %v", settings, err)
	}
	assertProfile("", "privacy-owner@example.test", false, true)
	assertProfile(otherID, "privacy-owner@example.test", false, true)
	assertProfile(id, "privacy-owner@example.test", false, true)

	u := User{ID: id}
	if err := s.FillProfile(ctx, &u); err != nil || u.FavoritesPublic == nil || *u.FavoritesPublic || u.EmailPublic == nil || !*u.EmailPublic {
		t.Fatalf("first-party profile must read current flags: %+v, err = %v", u, err)
	}
	if _, err := s.UpdatePrivacy(ctx, id, PrivacyPatch{EmailPublic: privacyBool(false)}); err != nil {
		t.Fatal(err)
	}
	assertProfile("", "", false, false)
	assertProfile(otherID, "", false, false)
	assertProfile(id, "privacy-owner@example.test", false, false)

	// 两个独立设置页同时提交不同字段时，后写者不能覆盖另一个请求未提交的字段。
	for iteration := 0; iteration < 12; iteration++ {
		if _, err := s.UpdatePrivacy(ctx, id, PrivacyPatch{FavoritesPublic: privacyBool(true), EmailPublic: privacyBool(false)}); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		var workers sync.WaitGroup
		for _, patch := range []PrivacyPatch{{FavoritesPublic: privacyBool(false)}, {EmailPublic: privacyBool(true)}} {
			workers.Add(1)
			go func(patch PrivacyPatch) {
				defer workers.Done()
				<-start
				_, err := s.UpdatePrivacy(ctx, id, patch)
				results <- err
			}(patch)
		}
		close(start)
		workers.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatalf("concurrent privacy update: %v", err)
			}
		}
		assertSettings(false, true)
	}
	other, err := s.Privacy(ctx, otherID)
	if err != nil || !other.FavoritesPublic || other.EmailPublic {
		t.Fatalf("owner changes touched another account: %+v, err = %v", other, err)
	}
	if _, err := s.Privacy(ctx, uuid.NewString()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown account must be ErrNoRows: %v", err)
	}
	if _, err := s.UpdatePrivacy(ctx, uuid.NewString(), PrivacyPatch{EmailPublic: privacyBool(true)}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown account update must be ErrNoRows: %v", err)
	}
}

func TestPrivacySchemaRequiresExplicitMigrationAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	s := &Store{DB: db}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// 改列名模拟上一版 schema，保留已有值；CheckSchema 必须只报错，不能偷偷补列。
	for _, column := range []string{"favorites_public", "email_public"} {
		t.Run(column, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, `ALTER TABLE auth.users RENAME COLUMN `+column+` TO privacy_test_missing`); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := db.ExecContext(ctx, `ALTER TABLE auth.users RENAME COLUMN privacy_test_missing TO `+column); err != nil {
					t.Errorf("restore privacy column: %v", err)
				}
			})
			if err := s.CheckSchema(ctx); err == nil || !strings.Contains(err.Error(), "-migrate") {
				t.Fatalf("missing %s must require explicit migration: %v", column, err)
			}
			var stillMissing bool
			if err := db.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='auth' AND table_name='users' AND column_name=$1)`, column).Scan(&stillMissing); err != nil || !stillMissing {
				t.Fatalf("CheckSchema changed the schema: missing=%v err=%v", stillMissing, err)
			}
		})
	}
}
