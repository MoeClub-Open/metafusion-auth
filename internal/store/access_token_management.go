package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type AccessTokenUpdate struct {
	Name   *string   `json:"name"`
	Scopes *[]string `json:"scopes"`
}

func (s *Store) UpdatePersonalAccessToken(ctx context.Context, actor *User, id string, in AccessTokenUpdate) error {
	if in.Scopes != nil {
		for _, scope := range *in.Scopes {
			if strings.TrimSpace(scope) == "mcp.connect" {
				return fmt.Errorf("forbidden_pat_scope")
			}
		}
	}
	return s.updateAccessToken(ctx, actor, id, "personal", in)
}

func (s *Store) UpdateMCPToken(ctx context.Context, actor *User, id string, scopes []string) error {
	scopes = append([]string{"mcp.connect"}, scopes...)
	return s.updateAccessToken(ctx, actor, id, "mcp", AccessTokenUpdate{Scopes: &scopes})
}

// A row lock prevents concurrent edits from restoring scopes another request removed.
// Editing metadata never changes the secret, expiry or revocation state.
func (s *Store) updateAccessToken(ctx context.Context, actor *User, id, purpose string, in AccessTokenUpdate) error {
	if actor == nil || actor.ID == "" || actor.IsThirdParty() || actor.TokenUse == "pat" {
		return fmt.Errorf("forbidden")
	}
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("token_not_found")
	}
	if in.Name == nil && in.Scopes == nil {
		return fmt.Errorf("invalid_request")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		var name string
		var previous []string
		err := tx.QueryRowContext(ctx, "SELECT name,scopes FROM auth.personal_access_tokens WHERE id=$1 AND user_id=$2 AND purpose=$3 AND deleted_at IS NULL FOR UPDATE", id, actor.ID, purpose).Scan(&name, pq.Array(&previous))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("token_not_found")
		}
		if err != nil {
			return err
		}
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
			if name == "" || utf8.RuneCountInString(name) > patNameMaxRunes {
				return fmt.Errorf("invalid_token_name")
			}
		}
		scopes := previous
		if in.Scopes != nil {
			var err error
			scopes, err = NormalizePATScopes(*in.Scopes, actor)
			if err != nil {
				return err
			}
			if purpose == "mcp" {
				for _, scope := range scopes {
					if !slices.Contains(previous, scope) {
						return fmt.Errorf("scope_expansion_requires_authorization")
					}
				}
			}
		}
		_, err = tx.ExecContext(ctx, "UPDATE auth.personal_access_tokens SET name=$1,scopes=$2 WHERE id=$3 AND user_id=$4 AND purpose=$5 AND deleted_at IS NULL", name, pq.Array(scopes), id, actor.ID, purpose)
		return err
	})
}

// Remove from the user's list and revoke atomically. Retain the row for audit
// attribution; introspection and quota calculations exclude deleted records.
func (s *Store) DeletePersonalAccessToken(ctx context.Context, user, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("token_not_found")
	}
	var found string
	err := s.DB.QueryRowContext(ctx, "UPDATE auth.personal_access_tokens SET deleted_at=coalesce(deleted_at,now()),revoked_at=coalesce(revoked_at,now()) WHERE id=$1 AND user_id=$2 AND purpose='personal' RETURNING id", id, user).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("token_not_found")
	}
	return err
}

// Possession of the complete delegated secret can revoke only that MCP
// credential. It cannot edit scopes, create credentials or affect personal keys.
// Unknown credentials return success, as in OAuth token revocation.
func (s *Store) RevokeMCPTokenByValue(ctx context.Context, token string) (owner, id string, err error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, PATPrefix) || len(token) != len(PATPrefix)+patSecretChars {
		return "", "", nil
	}
	err = s.DB.QueryRowContext(ctx, "UPDATE auth.personal_access_tokens SET revoked_at=coalesce(revoked_at,now()) WHERE token_hash=$1 AND purpose='mcp' RETURNING user_id,id", HashPersonalAccessToken(token)).Scan(&owner, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return
}
