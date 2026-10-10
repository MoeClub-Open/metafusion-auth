package handler

import (
	"github.com/MoeclubM/metafusion-auth/internal/audit"
	"github.com/MoeclubM/metafusion-auth/internal/store"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPATBusinessIdentityDoesNotBecomeSessionOrExpandScopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakePATs{plain: testPATPlain, principal: &store.PATPrincipal{UserID: "owner", Username: "user", TokenID: testPATID, Groups: []string{"admin"}, Permissions: []string{"auth.audit.read"}}}
	h := &Handler{tokens: fake}
	r := gin.New()
	r.Use(h.identity())
	r.GET("/api/admin/allowed", requirePermission("auth.audit.read"), func(c *gin.Context) {
		if h.auditActor(c).CredentialType != audit.CredentialPAT {
			t.Error("PAT attributed as session")
		}
		c.Status(200)
	})
	r.GET("/api/admin/denied", requirePermission("auth.users.manage"), func(c *gin.Context) { c.Status(200) })
	r.GET("/api/auth/me", requireUser(), func(c *gin.Context) { c.Status(200) })
	for _, tc := range []struct {
		path string
		code int
	}{{"/api/admin/allowed", 200}, {"/api/admin/denied", 403}, {"/api/auth/me", 401}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+testPATPlain)
		r.ServeHTTP(w, req)
		if w.Code != tc.code {
			t.Fatalf("%s: got %d want %d", tc.path, w.Code, tc.code)
		}
	}
}
