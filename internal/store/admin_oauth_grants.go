package store

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type AdminOAuthGrant struct {
	UserID     string     `json:"user_id"`
	Username   string     `json:"username"`
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	Scopes     []string   `json:"scopes"`
	GrantedAt  *time.Time `json:"granted_at"`
	UpdatedAt  *time.Time `json:"updated_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	TokenCount int        `json:"token_count"`
	Active     bool       `json:"active"`
}

// The self-service and administrator lists share the same projection. Consent
// history remains visible after a grant is revoked; it cannot authorize requests.
const oauthGrantProjection = `WITH consent AS (
 SELECT DISTINCT ON (subject_user_id,client_id) subject_user_id user_id,client_id,scopes,created_at
 FROM auth.oauth_audit WHERE subject_user_id IS NOT NULL AND action IN ('consent_allow','trusted_allow')
 ORDER BY subject_user_id,client_id,created_at DESC,id DESC
), live AS (
 SELECT t.* FROM auth.oauth_tokens t JOIN auth.oauth_clients c ON c.id=t.client_id JOIN auth.users u ON u.id=t.user_id
 WHERE t.expires_at>now() AND NOT c.disabled AND NOT u.banned
), tokens AS (
 SELECT user_id,client_id,count(*) token_count,min(created_at) created_at,max(created_at) updated_at,max(expires_at) expires_at
 FROM live GROUP BY user_id,client_id
), scopes AS (
 SELECT user_id,client_id,array_agg(DISTINCT sc ORDER BY sc) scopes
 FROM live CROSS JOIN LATERAL unnest(string_to_array(scope,' ')) sc WHERE sc<>'' GROUP BY user_id,client_id
), pairs AS (
 SELECT user_id,client_id FROM consent UNION SELECT user_id,client_id FROM tokens
), grants AS (
 SELECT p.user_id,u.username,p.client_id,COALESCE(NULLIF(c.name,''),p.client_id) client_name,
 COALESCE(s.scopes,a.scopes,'{}'::text[]) scopes,COALESCE(a.created_at,t.created_at) granted_at,
 GREATEST(a.created_at,t.updated_at) updated_at,t.expires_at,COALESCE(t.token_count,0) token_count,
 COALESCE(t.token_count,0)>0 active
 FROM pairs p JOIN auth.users u ON u.id=p.user_id LEFT JOIN auth.oauth_clients c ON c.id=p.client_id
 LEFT JOIN consent a ON a.user_id=p.user_id AND a.client_id=p.client_id
 LEFT JOIN tokens t ON t.user_id=p.user_id AND t.client_id=p.client_id
 LEFT JOIN scopes s ON s.user_id=p.user_id AND s.client_id=p.client_id
) `

func (s *Store) listOAuthGrantRecords(ctx context.Context, f AdminRecordFilter, paginated bool) (AdminPage[AdminOAuthGrant], error) {
	out := AdminPage[AdminOAuthGrant]{Items: []AdminOAuthGrant{}, Limit: f.Limit, Offset: f.Offset}
	where, args := "true", []any{}
	add := func(condition string, value any) {
		args = append(args, value)
		where += fmt.Sprintf(" AND "+condition, len(args))
	}
	if f.OwnerID != "" {
		add("user_id=$%d", f.OwnerID)
	}
	if f.ClientID != "" {
		add("client_id=$%d", f.ClientID)
	}
	switch f.Status {
	case "":
	case "active":
		where += " AND active"
	case "inactive":
		where += " AND NOT active"
	default:
		return out, fmt.Errorf("invalid_status")
	}
	if err := s.DB.QueryRowContext(ctx, oauthGrantProjection+"SELECT count(*) FROM grants WHERE "+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	query := oauthGrantProjection + "SELECT user_id,username,client_id,client_name,scopes,granted_at,updated_at,expires_at,token_count,active FROM grants WHERE " + where + " ORDER BY active DESC,updated_at DESC NULLS LAST,client_id,user_id"
	if paginated {
		args = append(args, f.Limit, f.Offset)
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var g AdminOAuthGrant
		if err = rows.Scan(&g.UserID, &g.Username, &g.ClientID, &g.ClientName, pq.Array(&g.Scopes), &g.GrantedAt, &g.UpdatedAt, &g.ExpiresAt, &g.TokenCount, &g.Active); err != nil {
			return out, err
		}
		if g.Scopes == nil {
			g.Scopes = []string{}
		}
		out.Items = append(out.Items, g)
	}
	out.HasMore = out.Offset+len(out.Items) < out.Total
	return out, rows.Err()
}

func (s *Store) ListAdminOAuthGrants(ctx context.Context, f AdminRecordFilter, actor *User) (AdminPage[AdminOAuthGrant], error) {
	if !canManageOAuth(actor) {
		return AdminPage[AdminOAuthGrant]{}, fmt.Errorf("forbidden")
	}
	f, err := f.normalized()
	if err != nil {
		return AdminPage[AdminOAuthGrant]{}, err
	}
	return s.listOAuthGrantRecords(ctx, f, true)
}

func (s *Store) RevokeAdminOAuthGrant(ctx context.Context, actor *User, userID, clientID string) (int, error) {
	if !canManageOAuth(actor) {
		return 0, fmt.Errorf("forbidden")
	}
	f, err := (AdminRecordFilter{OwnerID: userID, ClientID: clientID}).normalized()
	if err != nil {
		return 0, err
	}
	if f.OwnerID == "" || f.ClientID == "" {
		return 0, fmt.Errorf("invalid_revoke_target")
	}
	return s.revokeOAuthGrant(ctx, f.OwnerID, f.ClientID, actor.ID, "admin_grant")
}
