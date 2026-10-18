package store

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/testutil"
	"github.com/google/uuid"
)

func TestAuditMultiValueFiltersAreDriverValues(t *testing.T) {
	where, args := auditWhere(AuditQuery{Services: []string{"catalog", "auth"}, Actions: []string{"workspace.created", "session.login"}})
	if where != " WHERE service = ANY($1) AND action = ANY($2)" || len(args) != 2 {
		t.Fatalf("unexpected query: %s %#v", where, args)
	}
	for _, arg := range args {
		if _, err := driver.DefaultParameterConverter.ConvertValue(arg); err != nil {
			t.Fatalf("audit filters cannot be bound by database/sql: %v", err)
		}
	}
}

func TestAuditMultiValueFiltersAgainstPostgres(t *testing.T) {
	s := &Store{DB: testutil.Database(t)}
	ctx := context.Background()
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	actor, other := uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		if _, err := s.DB.Exec("DELETE FROM audit.audit_log WHERE actor_user_id IN ($1,$2)", actor, other); err != nil {
			t.Error(err)
		}
	})
	for _, row := range []struct{ service, action, result, actor string }{
		{"catalog", "workspace.created", "success", actor},
		{"catalog", "workspace.created", "success", actor},
		{"auth", "session.login", "success", actor},
		{"auth", "session.login", "failure", actor},
		{"storage", "asset.upload_completed", "success", actor},
		{"catalog", "workspace.created", "success", other},
	} {
		if _, err := s.DB.ExecContext(ctx, "INSERT INTO audit.audit_log(id,service,action,result,actor_user_id) VALUES($1,$2,$3,$4,$5)", uuid.NewString(), row.service, row.action, row.result, row.actor); err != nil {
			t.Fatal(err)
		}
	}
	q := AuditQuery{Services: []string{"catalog", "auth"}, Actions: []string{"workspace.created", "session.login"}, Result: "success", ActorUserID: actor, Page: 1, PerPage: 2}
	first, err := s.ListAuditLogs(ctx, q)
	if err != nil || first.Total != 3 || len(first.Items) != 2 {
		t.Fatalf("first page: %+v %v", first, err)
	}
	q.Page = 2
	second, err := s.ListAuditLogs(ctx, q)
	if err != nil || second.Total != 3 || len(second.Items) != 1 {
		t.Fatalf("second page: %+v %v", second, err)
	}
	for _, row := range append(first.Items, second.Items...) {
		if row.ActorUserID != actor || row.Result != "success" {
			t.Fatalf("scope or outcome escaped filter: %+v", row)
		}
	}
	q.Page, q.Services = 1, []string{"community"}
	empty, err := s.ListAuditLogs(ctx, q)
	if err != nil || empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatalf("no matches: %+v %v", empty, err)
	}
}
