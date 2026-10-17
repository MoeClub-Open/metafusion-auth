package store

import (
	"context"
	"fmt"
)

// PrivacySettings 是账号自己的展示偏好，不授予其他账号任何写入能力。
type PrivacySettings struct {
	FavoritesPublic bool `json:"favorites_public"`
	EmailPublic     bool `json:"email_public"`
}

// PrivacyPatch 只更新明确提供的字段，避免两个独立开关并发保存时覆盖彼此。
type PrivacyPatch struct {
	FavoritesPublic *bool `json:"favorites_public"`
	EmailPublic     *bool `json:"email_public"`
}

func (s *Store) Privacy(ctx context.Context, userID string) (PrivacySettings, error) {
	var out PrivacySettings
	err := s.DB.QueryRowContext(ctx,
		`SELECT favorites_public,email_public FROM auth.users WHERE id=$1`, userID).
		Scan(&out.FavoritesPublic, &out.EmailPublic)
	return out, err
}

func (s *Store) UpdatePrivacy(ctx context.Context, userID string, patch PrivacyPatch) (PrivacySettings, error) {
	var out PrivacySettings
	if patch.FavoritesPublic == nil && patch.EmailPublic == nil {
		return out, fmt.Errorf("invalid_payload")
	}
	// COALESCE 读取当前行的未修改字段；单条 UPDATE 在行锁下原子完成。
	err := s.DB.QueryRowContext(ctx,
		`UPDATE auth.users SET favorites_public=COALESCE($2::boolean,favorites_public),
		 email_public=COALESCE($3::boolean,email_public) WHERE id=$1
		 RETURNING favorites_public,email_public`, userID, patch.FavoritesPublic, patch.EmailPublic).
		Scan(&out.FavoritesPublic, &out.EmailPublic)
	return out, err
}
