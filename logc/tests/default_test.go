package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/logc/routers"
	"ydsz-trace/pkg/config"

	. "github.com/smartystreets/goconvey/convey"
)

// TestMainRoute 根路径测试
func TestMainRoute(t *testing.T) {
	cfg := config.NewDefault()
	r := routers.SetupRouter(cfg)

	Convey("Subject: Test Station Endpoint\n", t, func() {
		Convey("GET / Should Return 200", func() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, 200)
			So(w.Body.Len(), ShouldBeGreaterThan, 0)
		})

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
	})
}
