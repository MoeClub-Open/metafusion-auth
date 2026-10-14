package store

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
)

// retiredPermissionCodes 是已从 PermissionCatalog / 系统组种子删除、必须从存量数据清掉的码。
// 种子回填只加不减（见 backfillPermissions），所以删码不能靠启动回填，必须在 Migrate 里显式剥离。
// 禁止把死码映射到别的权限：清掉即止，不做兼容别名。
var retiredPermissionCodes = []string{"catalog.import.submit"}

// stripPermissionCodes 从权限/scopes 数组去掉 retired，保持相对顺序；全删时回空切片（非 nil）。
func stripPermissionCodes(cur []string, retired ...string) []string {
	if len(cur) == 0 || len(retired) == 0 {
		if cur == nil {
			return nil
		}
		out := make([]string, len(cur))
		copy(out, cur)
		return out
	}
	drop := make(map[string]bool, len(retired))
	for _, c := range retired {
		drop[c] = true
	}
	out := make([]string, 0, len(cur))
	for _, c := range cur {
		if drop[c] {
			continue
		}
		out = append(out, c)
	}
	return out
}

func stripArraySQL(column string) string {
	return `COALESCE((
		SELECT array_agg(p ORDER BY ord)
		FROM unnest(` + column + `) WITH ORDINALITY AS t(p, ord)
		WHERE NOT (p = ANY($1::text[]))
	), '{}'::text[])`
}

// stripRetiredPermissions 从全部权限组与 PAT scopes 剥离 retiredPermissionCodes（幂等）。
// 不改 oauth_audit 等历史审计行。
func stripRetiredPermissions(ctx context.Context, tx *sql.Tx) error {
	if len(retiredPermissionCodes) == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE auth.groups SET permissions = `+stripArraySQL("permissions")+` WHERE permissions && $1::text[]`,
		pq.Array(retiredPermissionCodes),
	); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE auth.personal_access_tokens SET scopes = `+stripArraySQL("scopes")+` WHERE scopes && $1::text[]`,
		pq.Array(retiredPermissionCodes),
	)
	return err
}
