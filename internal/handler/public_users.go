package handler

// 公开账号资料：GET /api/users/:id（匿名可读）。
//
// 前端用户主页读的就是这条路径（frontend/src/app/users/[id]/page.tsx）。资料本身公开，
// 但 email 只在本人或用户主动公开时带出——网关放行匿名，不代表字段也公开：
// 判定在 store.PublicProfile 里做，这里只负责把当前身份传进去（没有身份就是空串）。

import (
	"github.com/gin-gonic/gin"

	"github.com/MoeclubM/metafusion-auth/internal/store"
)

func (h *Handler) registerPublicUsers(api *gin.RouterGroup) {
	api.GET("/users/:id", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		viewer := ""
		// 机器凭据与第三方 subject 不等于用户亲自查看私有资料。
		// OAuth 的 email 授权由 /oauth/userinfo 按 scope 单独处理。
		if u := currentUser(c); u != nil && u.TokenUse == store.TokenUseSession {
			viewer = u.ID
		}
		p, err := h.store.PublicProfile(c.Request.Context(), c.Param("id"), viewer)
		respond(c, p, err)
	})
}
