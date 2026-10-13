package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSessionCookieAlsoWritesReadableHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/auth/login", nil)
	setSessionCookie(c, "tok", 3600)
	cookies := strings.Join(w.Header().Values("Set-Cookie"), "\n")
	if !strings.Contains(cookies, "mf_session=tok") || !strings.Contains(cookies, "mf_has_session=1") {
		t.Fatal(cookies)
	}
	for _, line := range w.Header().Values("Set-Cookie") {
		if strings.HasPrefix(line, "mf_has_session=") && (strings.Contains(line, "HttpOnly") || strings.Contains(line, "tok")) {
			t.Fatal("hint must be readable and carry no credential", line)
		}
	}
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/auth/logout", nil)
	clearSessionCookie(c)
	if !strings.Contains(strings.Join(w.Header().Values("Set-Cookie"), "\n"), "mf_has_session=; Path=/; Max-Age=0") {
		t.Fatal("hint not cleared", w.Header().Values("Set-Cookie"))
	}
}
