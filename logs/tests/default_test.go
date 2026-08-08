package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/logs/routers"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	. "github.com/smartystreets/goconvey/convey"
)

// TestMainRoute 根路径测试
func TestMainRoute(t *testing.T) {
	cfg := config.NewDefault()
	sessionMgr := session.NewManager()
	r := routers.SetupRouter(cfg, sessionMgr)

	Convey("Subject: Test Station Endpoint\n", t, func() {
		Convey("GET /health Should Return 200", func() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/health", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, 200)
		})

		Convey("GET /ready Should Return 200", func() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/ready", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, 200)
		})

		Convey("未登录访问 /client/queryAll Should Return 401", func() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/client/queryAll", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, 401)
		})
	})
}
