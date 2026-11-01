package handler

import (
	"fmt"
	"time"

	"github.com/MoeclubM/metafusion-auth/internal/audit"
	"github.com/gin-gonic/gin"
)

type logCleanupInput struct {
	Kind    string `json:"kind" form:"kind"`
	Before  string `json:"before" form:"before"`
	Service string `json:"service" form:"service"`
}

func (in logCleanupInput) cutoff() (time.Time, error) {
	before, err := time.Parse("2006-01-02", in.Before)
	if err != nil || before.After(time.Now().UTC().Truncate(24*time.Hour)) || (in.Kind != "audit" && in.Kind != "statistics") || !validLogService(in.Service) {
		return time.Time{}, fmt.Errorf("invalid_query: cleanup")
	}
	return before, nil
}

func validLogService(service string) bool {
	return service == "" || service == "catalog" || service == "auth" || service == "community" || service == "storage"
}

func (h *Handler) registerAPIStatistics(api *gin.RouterGroup) {
	api.GET("/admin/api-statistics", requirePermission(permissionAuditRead), func(c *gin.Context) {
		from, e1 := time.Parse("2006-01-02", c.DefaultQuery("from", time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")))
		to, e2 := time.Parse("2006-01-02", c.DefaultQuery("to", time.Now().UTC().Format("2006-01-02")))
		service := c.Query("service")
		if e1 != nil || e2 != nil || from.After(to) || to.Sub(from) >= 90*24*time.Hour || !validLogService(service) {
			respond(c, nil, fmt.Errorf("invalid_query: range"))
			return
		}
		items, err := h.store.APIStatistics(c.Request.Context(), from, to, service)
		respond(c, gin.H{"items": items, "flush_interval_seconds": 30}, err)
	})
	api.GET("/admin/log-cleanup", requirePermission("auth.audit.manage"), func(c *gin.Context) {
		var in logCleanupInput
		if err := c.ShouldBindQuery(&in); err != nil {
			respond(c, nil, fmt.Errorf("invalid_query: cleanup"))
			return
		}
		before, err := in.cutoff()
		if err != nil {
			respond(c, nil, err)
			return
		}
		count, err := h.store.PreviewLogCleanup(c.Request.Context(), in.Kind, before, in.Service)
		respond(c, gin.H{"count": count}, err)
	})
	api.POST("/admin/log-cleanup", requirePermission("auth.audit.manage"), func(c *gin.Context) {
		var in logCleanupInput
		if err := c.ShouldBindJSON(&in); err != nil {
			respond(c, nil, fmt.Errorf("invalid_payload"))
			return
		}
		before, err := in.cutoff()
		if err != nil {
			respond(c, nil, err)
			return
		}
		removed, err := h.store.CleanupLogs(c.Request.Context(), in.Kind, before, in.Service)
		if err == nil {
			audit.Describe(c, audit.Detail{TargetType: "logs", TargetID: in.Kind, Changes: map[string]any{"before": in.Before, "service": in.Service, "removed": removed}})
		}
		respond(c, gin.H{"removed": removed}, err)
	})
}
