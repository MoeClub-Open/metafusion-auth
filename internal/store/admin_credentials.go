package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type AdminRecordFilter struct {
	OwnerID  string
	ClientID string
	Kind     string
	Status   string
	Limit    int
	Offset   int
}

type AdminPage[T any] struct {
	Items   []T  `json:"items"`
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

func (f AdminRecordFilter) normalized() (AdminRecordFilter, error) {
	f.OwnerID, f.ClientID = strings.TrimSpace(f.OwnerID), strings.TrimSpace(f.ClientID)
	if f.OwnerID != "" {
		if _, err := uuid.Parse(f.OwnerID); err != nil {
			return f, fmt.Errorf("invalid_owner_id")
		}
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		return f, fmt.Errorf("invalid_offset")
	}
	return f, nil
}

type AdminCredential struct {
	PersonalAccessToken
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
}

// Only display metadata is selected; neither secret nor token hash reaches a DTO.
func (s *Store) ListAdminCredentials(ctx context.Context, f AdminRecordFilter, actor *User) (AdminPage[AdminCredential], error) {
	out := AdminPage[AdminCredential]{Items: []AdminCredential{}}
	if !Can(actor, "auth.users.manage") {
		return out, fmt.Errorf("forbidden")
	}
	var err error
	f, err = f.normalized()
	if err != nil {
		return out, err
	}
	out.Limit, out.Offset = f.Limit, f.Offset
	where, args := "t.deleted_at IS NULL", []any{}
	add := func(condition string, value any) {
		args = append(args, value)
		where += fmt.Sprintf(" AND "+condition, len(args))
	}
	if f.OwnerID != "" {
		add("t.user_id=$%d", f.OwnerID)
	}
	if f.Kind != "" {
		purpose := map[string]string{"pat": "personal", "mcp": "mcp"}[f.Kind]
		if purpose == "" {
			return out, fmt.Errorf("invalid_kind")
		}
		add("t.purpose=$%d", purpose)
	}
	switch f.Status {
	case "":
	case "active":
		where += " AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at>now()) AND NOT u.banned"
	case "revoked":
		where += " AND t.revoked_at IS NOT NULL"
	case "expired":
		where += " AND t.revoked_at IS NULL AND t.expires_at<=now()"
	case "disabled":
		where += " AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at>now()) AND u.banned"
	default:
		return out, fmt.Errorf("invalid_status")
	}
	from := " FROM auth.personal_access_tokens t JOIN auth.users u ON u.id=t.user_id WHERE " + where
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*)"+from, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, "SELECT t.id,t.name,t.token_prefix,t.scopes,t.expires_at,t.last_used_at,t.created_at,t.revoked_at,t.user_id,u.username,t.purpose,CASE WHEN t.revoked_at IS NOT NULL THEN 'revoked' WHEN t.expires_at<=now() THEN 'expired' WHEN u.banned THEN 'disabled' ELSE 'active' END"+from+fmt.Sprintf(" ORDER BY t.created_at DESC,t.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AdminCredential
		if err := rows.Scan(&item.ID, &item.Name, &item.TokenPrefix, pq.Array(&item.Scopes), &item.ExpiresAt, &item.LastUsedAt, &item.CreatedAt, &item.RevokedAt, &item.UserID, &item.Username, &item.Kind, &item.Status); err != nil {
			return out, err
		}
		if item.Scopes == nil {
			item.Scopes = []string{}
		}
		if item.Kind == "personal" {
			item.Kind = "pat"
		}
		item.Active = item.Status == "active"
		out.Items = append(out.Items, item)
	}
	out.HasMore = out.Offset+len(out.Items) < out.Total
	return out, rows.Err()
}

func (s *Store) adminCredentialOwner(ctx context.Context, id string, actor *User) (User, string, error) {
	var owner User
	var purpose string
	if !Can(actor, "auth.users.manage") || actor.TokenUse == "pat" {
		return owner, purpose, fmt.Errorf("forbidden")
	}
	if _, err := uuid.Parse(id); err != nil {
		return owner, purpose, fmt.Errorf("token_not_found")
	}
	err := s.DB.QueryRowContext(ctx, "SELECT user_id,purpose FROM auth.personal_access_tokens WHERE id=$1 AND deleted_at IS NULL", id).Scan(&owner.ID, &purpose)
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("token_not_found")
	}
	return owner, purpose, err
}

func (s *Store) UpdateAdminCredential(ctx context.Context, actor *User, id string, in AccessTokenUpdate) error {
	owner, purpose, err := s.adminCredentialOwner(ctx, id, actor)
	if err != nil {
		return err
	}
	if err = s.WithAccess(ctx, &owner); err != nil {
		return err
	}
	// Both the manager and owner must hold every delegated operation. The shared
	// updater retains PAT restrictions and MCP consent narrowing.
	if purpose == "mcp" && in.Scopes != nil {
		scopes := append([]string{"mcp.connect"}, (*in.Scopes)...)
		in.Scopes = &scopes
	}
	if in.Scopes != nil {
		if _, err = NormalizePATScopes(*in.Scopes, actor); err != nil {
			return err
		}
	}
	if purpose == "personal" {
		return s.UpdatePersonalAccessToken(ctx, &owner, id, in)
	}
	if purpose != "mcp" {
		return fmt.Errorf("invalid_kind")
	}
	return s.updateAccessToken(ctx, &owner, id, purpose, in)
}

func (s *Store) RevokeAdminCredential(ctx context.Context, actor *User, id string) error {
	owner, purpose, err := s.adminCredentialOwner(ctx, id, actor)
	if err != nil {
		return err
	}
	return s.revokeAccessToken(ctx, owner.ID, id, purpose)
}

func (s *Store) DeleteAdminCredential(ctx context.Context, actor *User, id string) error {
	owner, purpose, err := s.adminCredentialOwner(ctx, id, actor)
	if err != nil {
		return err
	}
	return s.deleteAccessToken(ctx, owner.ID, id, purpose)
}

type AdminOAuthToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Username   string     `json:"username"`
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	Scope      string     `json:"scope"`
	CreatedAt  *time.Time `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	Active     bool       `json:"active"`
	Status     string     `json:"status"`
}

func (s *Store) ListAdminOAuthTokens(ctx context.Context, f AdminRecordFilter, actor *User) (AdminPage[AdminOAuthToken], error) {
	out := AdminPage[AdminOAuthToken]{Items: []AdminOAuthToken{}}
	if !canManageOAuth(actor) {
		return out, fmt.Errorf("forbidden")
	}
	var err error
	f, err = f.normalized()
	if err != nil {
		return out, err
	}
	out.Limit, out.Offset = f.Limit, f.Offset
	where, args := "true", []any{}
	add := func(condition string, value any) {
		args = append(args, value)
		where += fmt.Sprintf(" AND "+condition, len(args))
	}
	if f.OwnerID != "" {
		add("t.user_id=$%d", f.OwnerID)
	}
	if f.ClientID != "" {
		add("t.client_id=$%d", f.ClientID)
	}
	switch f.Status {
	case "":
	case "active":
		where += " AND t.expires_at>now() AND NOT c.disabled AND NOT u.banned"
	case "expired":
		where += " AND t.expires_at<=now()"
	case "disabled":
		where += " AND t.expires_at>now() AND (c.disabled OR u.banned)"
	default:
		return out, fmt.Errorf("invalid_status")
	}
	from := " FROM auth.oauth_tokens t JOIN auth.users u ON u.id=t.user_id JOIN auth.oauth_clients c ON c.id=t.client_id WHERE " + where
	if err = s.DB.QueryRowContext(ctx, "SELECT count(*)"+from, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, "SELECT t.id,t.user_id,u.username,t.client_id,c.name,t.scope,t.created_at,t.expires_at,(t.expires_at>now() AND NOT c.disabled AND NOT u.banned),CASE WHEN t.expires_at<=now() THEN 'expired' WHEN c.disabled OR u.banned THEN 'disabled' ELSE 'active' END"+from+fmt.Sprintf(" ORDER BY t.created_at DESC NULLS LAST,t.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AdminOAuthToken
		if err = rows.Scan(&item.ID, &item.UserID, &item.Username, &item.ClientID, &item.ClientName, &item.Scope, &item.CreatedAt, &item.ExpiresAt, &item.Active, &item.Status); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	out.HasMore = out.Offset+len(out.Items) < out.Total
	return out, rows.Err()
}

func (s *Store) DeleteAdminOAuthToken(ctx context.Context, actor *User, id string) error {
	if !canManageOAuth(actor) {
		return fmt.Errorf("forbidden")
	}
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("token_not_found")
	}
	var issued revokedToken
	err := s.write(ctx, func(tx *sql.Tx) error {
		var owner, client string
		err := tx.QueryRowContext(ctx, "DELETE FROM auth.oauth_tokens WHERE id=$1 RETURNING user_id,client_id,jti,expires_at", id).Scan(&owner, &client, &issued.jti, &issued.exp)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("token_not_found")
		}
		if err != nil {
			return err
		}
		return recordOAuthAuditTx(ctx, tx, actor.ID, owner, client, OAuthActionTokensRevoked, nil, "admin_token:"+id)
	})
	if err == nil {
		revokeIssued(s.Tokens, []revokedToken{issued})
	}
	return err
}
