package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"ydsz-trace/pkg/session"
)

func init() { gin.SetMode(gin.TestMode) }

func ctxWithSession(t *testing.T, mutate func(c *gin.Context)) *gin.Context {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/x", nil)
	mgr := session.NewManager()
	mgr.Middleware()(c)
	c.Request.Header.Set("Cookie", w.Header().Get("Set-Cookie"))
	mgr.Middleware()(c)
	if mutate != nil {
		mutate(c)
	}
	return c
}

func TestSetRoleThenRead(t *testing.T) {
	c := ctxWithSession(t, func(c *gin.Context) {
		SetRoleToSession(c, RoleAdmin)
	})

	if got := RoleFromSession(c); got != RoleAdmin {
		t.Fatalf("RoleFromSession got %v want Admin", got)
	}
	if got := TenantFromSession(c); got != "default" {
		t.Fatalf("default tenant got %q want default", got)
	}
}

func TestRoleHierarchy(t *testing.T) {
	cases := []struct {
		have, min Role
		want      bool
	}{
		{RoleAdmin, RoleViewer, true},
		{RoleAdmin, RoleOperator, true},
		{RoleAdmin, RoleAdmin, true},
		{RoleOperator, RoleViewer, true},
		{RoleOperator, RoleOperator, true},
		{RoleOperator, RoleAdmin, false},
		{RoleViewer, RoleViewer, true},
		{RoleViewer, RoleOperator, false},
		{RoleViewer, RoleAdmin, false},
	}
	for _, tc := range cases {
		c := ctxWithSession(t, func(c *gin.Context) {
			SetRoleToSession(c, tc.have)
		})
		if got := HasRole(c, tc.min); got != tc.want {
			t.Fatalf("HasRole(have=%s, min=%s)=%v want %v", tc.have, tc.min, got, tc.want)
		}
	}
}

func TestRequireRole_GinMiddleware(t *testing.T) {
	// viewer accessing operator-gated endpoint should 403
	c := ctxWithSession(t, func(c *gin.Context) {
		SetRoleToSession(c, RoleViewer)
	})
	mw := RequireRole(RoleOperator)
	w := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w)
	c2.Request = c.Request
	// Simulate c2 hits a route gated by RequireRole
	c2.Set("session", session.Get(c))
	c2.Set("session_manager", session.GetManager(c))

	mw(c2)

	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer should be forbidden, got %d", w.Code)
	}
}

func TestRoleFromName_UnknownFallback(t *testing.T) {
	if got := RoleFromName("superuser"); got != RoleViewer {
		t.Fatalf("unknown role should fallback to viewer, got %v", got)
	}
}
