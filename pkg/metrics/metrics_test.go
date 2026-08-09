package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func init() { gin.SetMode(gin.TestMode) }

func TestGlobal(t *testing.T) {
	c := Global()
	if c == nil {
		t.Fatal("Global() returned nil")
	}
	if Global() != c {
		t.Error("Global() should return same instance")
	}
}

func TestQueryCounters(t *testing.T) {
	c := ensureFreshCollector(t)

	before := testutil.ToFloat64(c.queryTotal)
	c.QueryStarted()
	if got := testutil.ToFloat64(c.queryTotal); got != before+1 {
		t.Errorf("queryTotal got %v want %v", got, before+1)
	}

	c.QuerySucceeded(5 * time.Millisecond)
	if got := testutil.ToFloat64(c.querySuccess); got < 1 {
		t.Errorf("querySuccess got %v want >=1", got)
	}

	c.QueryFailed(2 * time.Millisecond)
	if got := testutil.ToFloat64(c.queryFailed); got < 1 {
		t.Errorf("queryFailed got %v want >=1", got)
	}
}

func TestUpdateClientStats(t *testing.T) {
	c := ensureFreshCollector(t)

	c.UpdateClientStats(10, 7)
	if got := testutil.ToFloat64(c.clientTotal); got != 10 {
		t.Errorf("clientTotal got %v want 10", got)
	}
	if got := testutil.ToFloat64(c.clientOnline); got != 7 {
		t.Errorf("clientOnline got %v want 7", got)
	}
}

func TestHandler_Endpoint(t *testing.T) {
	c := Global()
	handler := c.Handler()

	// 先打一条请求，使得 CounterVec / Histogram 产生样本
	c.QueryStarted()
	c.QuerySucceeded(5 * time.Millisecond)
	c.HTTPRequestRecordedWithMethod("GET", 200, 10*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code got %d want %d", resp.StatusCode, http.StatusOK)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("content-type got %q want text/plain", ct)
	}

	body := w.Body.String()
	for _, metric := range []string{
		namespace + "_queries_total",
		namespace + "_clients_total",
		namespace + "_http_requests_total",
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("body should contain %q\n%s", metric, body)
		}
	}
	// Go runtime collector should contribute
	if !strings.Contains(body, "go_gc") {
		t.Errorf("body should contain Go runtime gc metrics")
	}
}

func TestHandler_GinAdapter(t *testing.T) {
	c := Global()
	handler := c.GinHandler()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler(ctx)

	if w.Code != http.StatusOK {
		t.Errorf("gin adapter status got %d want %d", w.Code, http.StatusOK)
	}
}

func TestHTTPMetricsMiddleware(t *testing.T) {
	_ = Global() // ensure singleton initialized
	middleware := HTTPMetricsMiddleware()

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	middleware(ctx)

	if w.Code != http.StatusOK {
		t.Errorf("unexpected status code: %d", w.Code)
	}
}

// ensureFreshCollector 让指标从初始状态开始，便于断言增量。
func ensureFreshCollector(t *testing.T) *Collector {
	t.Helper()
	globalCollector = nil
	return Global()
}
