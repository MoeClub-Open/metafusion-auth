package store

import (
	"reflect"
	"strings"
	"testing"
)

func TestStripPermissionCodes(t *testing.T) {
	cases := []struct {
		name    string
		cur     []string
		retired []string
		want    []string
	}{
		{
			name:    "strips dead import submit, keeps order",
			cur:     []string{"catalog.entity.edit", "catalog.import.submit", "catalog.relation.edit"},
			retired: []string{"catalog.import.submit"},
			want:    []string{"catalog.entity.edit", "catalog.relation.edit"},
		},
		{
			name:    "idempotent when already clean",
			cur:     []string{"catalog.entity.edit", "catalog.relation.edit"},
			retired: []string{"catalog.import.submit"},
			want:    []string{"catalog.entity.edit", "catalog.relation.edit"},
		},
		{
			name:    "all retired yields empty slice",
			cur:     []string{"catalog.import.submit"},
			retired: []string{"catalog.import.submit"},
			want:    []string{},
		},
		{
			name:    "nil input stays nil",
			cur:     nil,
			retired: []string{"catalog.import.submit"},
			want:    nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripPermissionCodes(tc.cur, tc.retired...)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestRetiredImportSubmitAbsentFromSeedsAndCatalog(t *testing.T) {
	const dead = "catalog.import.submit"
	for _, item := range PermissionCatalog() {
		if item.Code == dead {
			t.Fatalf("PermissionCatalog still lists %s", dead)
		}
	}
	for _, g := range systemGroups() {
		for _, p := range g.perms {
			if p == dead {
				t.Fatalf("system group %s still seeds %s", g.code, dead)
			}
		}
		for _, text := range g.desc {
			if strings.Contains(text, "导入") || strings.Contains(text, "匯入") ||
				strings.Contains(text, "取り込み") || strings.Contains(strings.ToLower(text), "import") {
				t.Fatalf("system group %s description still mentions import: %q", g.code, text)
			}
		}
	}
	if len(retiredPermissionCodes) == 0 || retiredPermissionCodes[0] != dead {
		t.Fatalf("retiredPermissionCodes must list %s for Migrate strip", dead)
	}
}
