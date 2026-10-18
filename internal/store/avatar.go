package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
)

// Avatars are small account presentation data; their bytes never enter profile JSON or JWTs.
const avatarURLSQL = `COALESCE((SELECT '/api/users/' || a.user_id::text || '/avatar?v=' || a.version FROM auth.user_avatars a WHERE a.user_id=auth.users.id),'')`

func (s *Store) SetAvatar(ctx context.Context, userID string, data []byte) (string, error) {
	version := fmt.Sprintf("%x", sha256.Sum256(data))
	_, err := s.DB.ExecContext(ctx, `INSERT INTO auth.user_avatars(user_id,data,version) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET data=EXCLUDED.data,version=EXCLUDED.version`, userID, data, version)
	return "/api/users/" + userID + "/avatar?v=" + version, err
}

func (s *Store) RemoveAvatar(ctx context.Context, userID string) error {
	_, err := s.DB.ExecContext(ctx, "DELETE FROM auth.user_avatars WHERE user_id=$1", userID)
	return err
}

func (s *Store) Avatar(ctx context.Context, userID string) ([]byte, string, error) {
	if s.DB == nil {
		return nil, "", sql.ErrNoRows
	}
	var data []byte
	var version string
	err := s.DB.QueryRowContext(ctx, "SELECT data,version FROM auth.user_avatars WHERE user_id=$1", userID).Scan(&data, &version)
	return data, version, err
}
