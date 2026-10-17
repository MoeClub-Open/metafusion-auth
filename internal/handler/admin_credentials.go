package handler

import (
	"strconv"

	"github.com/MoeclubM/metafusion-auth/internal/audit"
	"github.com/MoeclubM/metafusion-auth/internal/store"
	"github.com/gin-gonic/gin"
)

func adminRecordFilter(c *gin.Context) (store.AdminRecordFilter, bool) {
	f := store.AdminRecordFilter{OwnerID: c.Query("owner_id"), ClientID: c.Query("client_id"), Kind: c.Query("kind"), Status: c.Query("status")}
	for key, target := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if raw := c.Query(key); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 0 || (key == "limit" && v == 0) {
				c.AbortWithStatusJSON(400, gin.H{"error": "invalid_" + key})
				return f, false
			}
			*target = v
		}
	}
	return f, true
}

func (h *Handler) registerAdminCredentials(api *gin.RouterGroup) {
	s := h.store
	keys := requirePermission("auth.users.manage")
	oauth := requirePermission("auth.oauth.manage")
	api.GET("/admin/credentials", keys, func(c *gin.Context) {
		f, ok := adminRecordFilter(c)
		if !ok {
			return
		}
		page, err := s.ListAdminCredentials(c.Request.Context(), f, currentUser(c))
		c.Header("Cache-Control", "no-store")
		respond(c, page, err)
	})
	api.PATCH("/admin/credentials/:id", keys, func(c *gin.Context) {
		var in store.AccessTokenUpdate
		if !body(c, &in) {
			return
		}
		err := s.UpdateAdminCredential(c.Request.Context(), currentUser(c), c.Param("id"), in)
		audit.Describe(c, audit.Detail{TargetType: "credential", TargetID: c.Param("id"), Changes: map[string]any{"name": in.Name, "scopes": in.Scopes}})
		respond(c, gin.H{"ok": true}, err)
	})
	api.POST("/admin/credentials/:id/revoke", keys, func(c *gin.Context) {
		err := s.RevokeAdminCredential(c.Request.Context(), currentUser(c), c.Param("id"))
		audit.Describe(c, audit.Detail{TargetType: "credential", TargetID: c.Param("id")})
		respond(c, gin.H{"ok": true}, err)
	})
	api.DELETE("/admin/credentials/:id", keys, func(c *gin.Context) {
		err := s.DeleteAdminCredential(c.Request.Context(), currentUser(c), c.Param("id"))
		audit.Describe(c, audit.Detail{TargetType: "credential", TargetID: c.Param("id")})
		respond(c, gin.H{"ok": true}, err)
	})
	api.GET("/admin/oauth/grants", oauth, func(c *gin.Context) {
		f, ok := adminRecordFilter(c)
		if !ok {
			return
		}
		page, err := s.ListAdminOAuthGrants(c.Request.Context(), f, currentUser(c))
		c.Header("Cache-Control", "no-store")
		respond(c, page, err)
	})
	api.POST("/admin/oauth/grants/revoke", oauth, func(c *gin.Context) {
		var in struct {
			UserID   string `json:"user_id"`
			ClientID string `json:"client_id"`
		}
		if !body(c, &in) {
			return
		}
		n, err := s.RevokeAdminOAuthGrant(c.Request.Context(), currentUser(c), in.UserID, in.ClientID)
		audit.Describe(c, audit.Detail{TargetType: "oauth_grant", TargetID: in.UserID + ":" + in.ClientID, Changes: map[string]any{"revoked": n}})
		respond(c, gin.H{"revoked": n}, err)
	})
	api.GET("/admin/oauth/tokens", oauth, func(c *gin.Context) {
		f, ok := adminRecordFilter(c)
		if !ok {
			return
		}
		page, err := s.ListAdminOAuthTokens(c.Request.Context(), f, currentUser(c))
		c.Header("Cache-Control", "no-store")
		respond(c, page, err)
	})
	api.DELETE("/admin/oauth/tokens/:id", oauth, func(c *gin.Context) {
		err := s.DeleteAdminOAuthToken(c.Request.Context(), currentUser(c), c.Param("id"))
		audit.Describe(c, audit.Detail{TargetType: "oauth_token", TargetID: c.Param("id")})
		respond(c, gin.H{"ok": true}, err)
	})
}
