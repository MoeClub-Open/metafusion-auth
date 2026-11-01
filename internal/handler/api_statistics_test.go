package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/store"
	"github.com/gin-gonic/gin"
)

func TestLogCleanupRequiresSeparatePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		user   *store.User
		status int
	}{
		{nil, 401},
		{&store.User{Permissions: []string{"auth.audit.read"}}, 403},
		{&store.User{Permissions: []string{"auth.audit.manage"}}, 400},
	} {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			if tc.user != nil {
				c.Set(userKey, tc.user)
			}
			c.Next()
		})
		h := &Handler{store: &store.Store{}}
		h.registerAPIStatistics(r.Group("/api"))
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/admin/log-cleanup", strings.NewReader(`{"kind":"audit","before":"2999-01-01"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("want %d got %d: %s", tc.status, w.Code, w.Body.String())
		}
	}
}
