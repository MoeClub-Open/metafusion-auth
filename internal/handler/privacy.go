package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MoeclubM/metafusion-auth/internal/audit"
	"github.com/MoeclubM/metafusion-auth/internal/store"
)

func (h *Handler) registerPrivacy(api *gin.RouterGroup) {
	// 隐私设置是账号安全操作。机器凭据、第三方授权和 ID token 均不能修改。
	session := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		u := currentUser(c)
		if u == nil || u.TokenUse != store.TokenUseSession {
			audit.Fail(c, "authentication_required")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication_required"})
			return
		}
		c.Next()
	}
	api.GET("/auth/privacy", session, func(c *gin.Context) {
		out, err := h.store.Privacy(c.Request.Context(), currentUser(c).ID)
		respond(c, out, err)
	})
	api.PATCH("/auth/privacy", session, func(c *gin.Context) {
		var raw map[string]json.RawMessage
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONBody)
		dec := json.NewDecoder(c.Request.Body)
		var extra any
		if err := dec.Decode(&raw); err != nil || dec.Decode(&extra) != io.EOF {
			audit.Fail(c, "invalid_payload")
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
			return
		}
		var patch store.PrivacyPatch
		valid := len(raw) > 0
		for key, value := range raw {
			var flag *bool
			if err := json.Unmarshal(value, &flag); err != nil || flag == nil {
				valid = false
				break
			}
			switch key {
			case "favorites_public":
				patch.FavoritesPublic = flag
			case "email_public":
				patch.EmailPublic = flag
			default:
				valid = false
			}
		}
		if !valid {
			audit.Fail(c, "invalid_payload")
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
			return
		}
		u := currentUser(c)
		out, err := h.store.UpdatePrivacy(c.Request.Context(), u.ID, patch)
		if err == nil {
			changes := map[string]any{"self_service": true}
			if patch.FavoritesPublic != nil {
				changes["favorites_public"] = *patch.FavoritesPublic
			}
			if patch.EmailPublic != nil {
				changes["email_public"] = *patch.EmailPublic
			}
			audit.Describe(c, audit.Detail{TargetType: "user", TargetID: u.ID, Changes: changes})
		}
		respond(c, out, err)
	})
}
