package handler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MoeclubM/metafusion-auth/internal/audit"
	"github.com/MoeclubM/metafusion-auth/internal/store"
)

// InternalTokenHeader 与目录/互动侧跨服务投递头逐字一致（X-Internal-Token）。
// credentials-status 用同一共享密钥（INTERNAL_API_TOKEN）证明服务身份。
const InternalTokenHeader = "X-Internal-Token"

// SetInternalToken 注入跨服务共享密钥。空串 = 服务身份 fail-closed（永不匹配）。
func (h *Handler) SetInternalToken(token string) {
	h.internalToken = strings.TrimSpace(token)
}

func (h *Handler) credentialsStatus(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("user_id"))
	if h.internalServiceAuthorized(c) {
		st, err := h.lookupCredentialsStatus(c.Request.Context(), userID)
		respond(c, st, err)
		return
	}
	u := currentUser(c)
	if u == nil {
		// 匿名不得探知账号是否存在，也不得看到 token_version / banned。
		audit.Fail(c, "authentication_required")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication_required"})
		return
	}
	if u.ID != userID {
		// 跨用户：无论目标是否存在，一律 403（防枚举）。
		audit.Fail(c, "forbidden")
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	st, err := h.lookupCredentialsStatus(c.Request.Context(), userID)
	respond(c, st, err)
}

func (h *Handler) internalServiceAuthorized(c *gin.Context) bool {
	want := h.internalToken
	if want == "" {
		return false
	}
	given := c.GetHeader(InternalTokenHeader)
	return subtle.ConstantTimeCompare([]byte(given), []byte(want)) == 1
}

func (h *Handler) lookupCredentialsStatus(ctx context.Context, userID string) (store.CredentialsStatus, error) {
	if h.credentialsStatusFn != nil {
		return h.credentialsStatusFn(ctx, userID)
	}
	return h.store.CredentialsStatus(ctx, userID)
}
