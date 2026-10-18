package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/MoeclubM/metafusion-auth/internal/store"
	"github.com/gin-gonic/gin"
)

func TestAuditActorUsesVerifiedCredentialKind(t *testing.T) {
	for _, kind := range []string{"session", "pat", "oauth"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(userKey, &store.User{ID: "actor", Username: "name", TokenUse: kind})
			got := (&Handler{}).auditActor(c)
			if got.CredentialType != kind || got.UserID != "actor" || got.Username != "name" {
				t.Fatalf("identity projection: %+v", got)
			}
		})
	}
}
