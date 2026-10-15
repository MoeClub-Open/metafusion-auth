package store

import (
	"context"
	"fmt"
)

// CheckSchema only reads schema metadata and columns; HTTP startup cannot run DDL
// or seed/backfill permissions. Existing instances must run auth-server -migrate.
func (s *Store) CheckSchema(ctx context.Context) error {
	checks := []string{
		`SELECT id,username,email,password_hash,banned,display_name,bio FROM auth.users LIMIT 0`,
		`SELECT token_hash,user_id,expires_at FROM auth.sessions LIMIT 0`,
		`SELECT id,secret_hash,name,redirect_uris,trusted,scopes,disabled,owner_user_id,description,homepage_url,verified,created_at FROM auth.oauth_clients LIMIT 0`,
		`SELECT code,client_id,user_id,redirect_uri,scope,expires_at,used,code_challenge,code_challenge_method FROM auth.oauth_codes LIMIT 0`,
		`SELECT token_hash,client_id,user_id,scope,expires_at,jti FROM auth.oauth_tokens LIMIT 0`,
		`SELECT id,actor_user_id,subject_user_id,client_id,action,scopes,detail,created_at FROM auth.oauth_audit LIMIT 0`,
		`SELECT key,value,updated_at FROM auth.instance_settings LIMIT 0`,
		`SELECT code,created_by,note,max_uses,used_count,revoked,expires_at,created_at FROM auth.invites LIMIT 0`,
		`SELECT invite_code,user_id,used_at FROM auth.invite_uses LIMIT 0`,
		`SELECT id,code,names,descriptions,permissions,is_system,sort_order,created_at FROM auth.groups LIMIT 0`,
		`SELECT user_id,group_id,granted_by,granted_at FROM auth.user_groups LIMIT 0`,
		`SELECT id,user_id,name,token_hash,token_prefix,scopes,expires_at,last_used_at,created_at,revoked_at,purpose,deleted_at FROM auth.personal_access_tokens LIMIT 0`,
		`SELECT dimension,key_hash,failures,expires_at FROM auth.login_failures LIMIT 0`,
		`SELECT id,occurred_at,service,action,actor_user_id,actor_username,credential_type,actor_ip,actor_user_agent,target_type,target_id,changes,result,error_code,request_method,route,http_status,request_id FROM audit.audit_log LIMIT 0`,
	}
	for _, query := range checks {
		rows, err := s.DB.QueryContext(ctx, query)
		if err != nil {
			return fmt.Errorf("run auth-server -migrate before HTTP startup: %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	var usernameUnique bool
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_index WHERE indexrelid=to_regclass('auth.users_username_lower_key')
		AND indrelid='auth.users'::regclass AND indisunique AND indisvalid
		AND indpred IS NULL AND indnkeyatts=1
		AND pg_get_expr(indexprs,indrelid)='lower(username)'
	)`).Scan(&usernameUnique); err != nil {
		return err
	}
	if !usernameUnique {
		return fmt.Errorf("username uniqueness migration required: run auth-server -migrate; see README collision recovery")
	}
	return nil
}
