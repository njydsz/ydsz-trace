package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGlobal(t *testing.T) {
	c := Global()
	if c == nil {
		t.Fatal("Global() returned nil")
	}
	// 单例
	if Global() != c {
		t.Error("Global() should return same instance")
	}
}

func TestQueryCounters(t *testing.T) {
	collector := &Collector{}
	collector.QueryStarted()
	collector.QuerySucceeded(100)
	collector.QueryFailed(50)

	if collector.queryTotal != 1 {
		t.Errorf("queryTotal got %d want 1", collector.queryTotal)
	}
	if collector.querySuccess != 1 {
		t.Errorf("querySuccess got %d want 1", collector.querySuccess)
	}
	if collector.queryFailure != 1 {
		t.Errorf("queryFailure got %d want 1", collector.queryFailure)
	}
}

func TestUpdateClientStats(t *testing.T) {
	collector := &Collector{}
	collector.UpdateClientStats(10, 7)
	if collector.clientTotal != 10 {
		t.Errorf("clientTotal got %d want 10", collector.clientTotal)
	}
	if collector.clientOnline != 7 {
		t.Errorf("clientOnline got %d want 7", collector.clientOnline)
	}
}

func TestHandler_Endpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := Global().Handler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/metrics", nil)

	handler(c)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code got %d want %d", resp.StatusCode, http.StatusOK)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("content-type got %q want text/plain", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ydsz_uptime_seconds") {
		t.Errorf("body should contain uptime metric, got: %s", string(body))
	}
	if !strings.Contains(string(body), "ydsz_queries_total") {
		t.Errorf("body should contain queries_total metric")
	}
}

func TestHTTPMetricsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	middleware := HTTPMetricsMiddleware()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	middleware(c)

	if w.Code != http.StatusOK && w.Code != 0 {
		t.Errorf("unexpected status code: %d", w.Code)
	}
}
