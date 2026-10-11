package store

import (
	"reflect"
	"testing"
)

func TestNewPATCannotDelegateManagement(t *testing.T) {
	admin := &User{ID: "admin-test", Permissions: []string{"*"}}
	for _, scope := range []string{"*", "auth.users.manage", "catalog.definitions.manage", "catalog.lifecycle.manage", "community.rate_limits.manage", "future.configuration.manage"} {
		if _, err := NormalizePATScopes([]string{"catalog.entity.edit", scope}, admin); err == nil || err.Error() != "forbidden_pat_scope" {
			t.Fatalf("management scope %q: %v", scope, err)
		}
	}
	for _, scope := range []string{"mcp.connect", "catalog.entity.edit", "auth.audit.read", "storage.asset.moderate"} {
		if got, err := NormalizePATScopes([]string{scope}, admin); err != nil || !reflect.DeepEqual(got, []string{scope}) {
			t.Fatalf("delegable scope %q: %v %v", scope, got, err)
		}
	}
	// New issuance policy does not retroactively revoke legacy delegation.
	if got := EffectivePATPermissions(admin, []string{"*"}); !reflect.DeepEqual(got, []string{"*"}) {
		t.Fatalf("legacy wildcard introspection changed: %v", got)
	}
	if got := EffectivePATPermissions(admin, []string{"catalog.lifecycle.manage"}); !reflect.DeepEqual(got, []string{"catalog.lifecycle.manage"}) {
		t.Fatalf("legacy management introspection changed: %v", got)
	}
}
