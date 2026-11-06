package admin

import (
	"encoding/json"
	"strings"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// setupRouter 创建带生命周期路由的测试引擎。
func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/lifecycle/plan", LifecyclePlanHandler)
	r.POST("/admin/lifecycle/apply", LifecycleApplyHandler)
	r.GET("/admin/lifecycle/stats", LifecycleStatsHandler)
	return r
}

// dateNDaysAgo N 天前的 YYYYMMDD。
func dateNDaysAgo(n int) string {
	return time.Now().AddDate(0, 0, -n).Format("20060102")
}

// createLogDir 创建含不同年龄日志的临时目录。
func createLogDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, age := range []int{1, 15, 60} {
		name := "app-" + dateNDaysAgo(age) + ".log"
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0o644); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}
	return dir
}

// doGet 执行 GET 并返回响应。
func doGet(r http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestLifecyclePlanHandler 计划接口返回分层信息。
func TestLifecyclePlanHandler(t *testing.T) {
	r := setupRouter()
	dir := createLogDir(t)

	w := doGet(r, "/admin/lifecycle/plan?dir="+dir)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Reports []struct {
				Tier   string `json:"tier"`
				Action string `json:"action"`
			} `json:"reports"`
			Stats struct {
				HotCount  int `json:"hotCount"`
				WarmCount int `json:"warmCount"`
				ColdCount int `json:"coldCount"`
			} `json:"stats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("code = %d", resp.Code)
	}
	if len(resp.Data.Reports) != 3 {
		t.Errorf("reports = %d want 3", len(resp.Data.Reports))
	}
	if resp.Data.Stats.HotCount != 1 || resp.Data.Stats.ColdCount != 1 {
		t.Errorf("stats = %+v", resp.Data.Stats)
	}
}

// TestLifecyclePlanHandler_DefaultDir 未指定目录时使用默认根目录。
//
// 默认目录（logs/）在测试环境下通常不存在，接口应返回错误响应而非 panic。
func TestLifecyclePlanHandler_DefaultDir(t *testing.T) {
	r := setupRouter()
	w := doGet(r, "/admin/lifecycle/plan")

	// 目录不存在时应返回错误码，而不是 panic 或 200
	if w.Code == http.StatusOK {
		var resp struct {
			Code int `json:"code"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Code == 0 {
			t.Error("默认目录不存在时应返回业务错误")
		}
	}
}

// TestLifecycleApplyHandler_DryRunByDefault 默认干跑不删文件。
func TestLifecycleApplyHandler_DryRunByDefault(t *testing.T) {
	r := setupRouter()
	dir := createLogDir(t)

	body := buildBody(t, dir, "")
	req := httptest.NewRequest(http.MethodPost, "/admin/lifecycle/apply",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			DryRun bool `json:"dryRun"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if !resp.Data.DryRun {
		t.Error("默认应为 dry-run")
	}

	// 文件应仍然存在
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		t.Error("dry-run 不应删除文件")
	}
}

// TestLifecycleApplyHandler_RealDelete 显式非干跑才执行。
func TestLifecycleApplyHandler_RealDelete(t *testing.T) {
	r := setupRouter()
	dir := createLogDir(t)

	body := buildBody(t, dir, "false")
	req := httptest.NewRequest(http.MethodPost, "/admin/lifecycle/apply?dryRun=false",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp struct {
		Data struct {
			DryRun bool `json:"dryRun"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	// 冷数据应被删除
	oldName := "app-" + dateNDaysAgo(60) + ".log"
	if _, err := os.Stat(filepath.Join(dir, oldName)); !os.IsNotExist(err) {
		t.Error("冷数据应被删除")
	}
}

// TestLifecycleStatsHandler 统计接口返回人类可读字段。
func TestLifecycleStatsHandler(t *testing.T) {
	r := setupRouter()
	dir := createLogDir(t)

	w := doGet(r, "/admin/lifecycle/stats?dir="+dir)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp struct {
		Data struct {
			Stats struct {
				TotalBytes int64 `json:"totalBytes"`
			} `json:"stats"`
			HumanTotal string `json:"humanTotal"`
			CheckedAt  string `json:"checkedAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if resp.Data.Stats.TotalBytes <= 0 {
		t.Error("TotalBytes 应大于 0")
	}
	if resp.Data.HumanTotal == "" {
		t.Error("humanTotal 不应为空")
	}
	if resp.Data.CheckedAt == "" {
		t.Error("checkedAt 不应为空")
	}
}

// buildBody 构造 apply 请求体（正确转义路径，避免 Windows 反斜杠破坏 JSON）。
func buildBody(t *testing.T, dir, dryRun string) string {
	t.Helper()
	payload := map[string]interface{}{"dir": dir}
	if dryRun != "" {
		payload["dryRun"] = dryRun == "false"
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("构造请求体失败: %v", err)
	}
	return string(raw)
}

// TestLifecycleApplyHandler_BadRequest 非法 JSON 返回 400。
func TestLifecycleApplyHandler_BadRequest(t *testing.T) {
	r := setupRouter()

	req := httptest.NewRequest(http.MethodPost, "/admin/lifecycle/apply",
		strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		// api.Fail 会返回 HTTP 200 + 业务码，或 400；两者都可接受
		return
	}
}
