// Package controllers_test 包含 logs 服务端的控制器集成测试（authz + client CRUD）。
//
// 使用 httptest.NewServer 配合 gin.Engine：
//   - 未登录访问受保护路由返回 401
//   - 已登录但缺少 CSRF 头的 POST 请求返回 403
//   - 已登录 + CSRF 头的 POST 请求正常处理
//   - 登录密码校验失败返回 401（不泄露用户名是否存在）
package controllers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ydsz-trace/logs/models"
	"ydsz-trace/logs/routers"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

const (
	testAdminUser = "testadmin"
	testAdminPass = "TestAdminPass_2026!"
)

// setupTestServer 启动一个带固定测试账号 + 临时 SQLite DB 的 Gin HTTP 测试服务器。
//
// 覆盖环境变量 YDSZ_ADMIN_USER / YDSZ_ADMIN_PASSWORD，确保登录测试使用已知凭据。
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)

	t.Setenv("YDSZ_ADMIN_USER", testAdminUser)
	t.Setenv("YDSZ_ADMIN_PASSWORD", testAdminPass)

	// 初始化临时 SQLite 测试数据库
	tmpDir, err := os.MkdirTemp("", "ydsz-test-*")
	if err != nil {
		t.Fatalf("create temp dir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "test.db")
	if err := models.InitDB(&models.SQLiteConfig{FilePath: dbPath}); err != nil {
		t.Fatalf("init test DB failed: %v", err)
	}
	t.Cleanup(func() {
		if models.DB != nil {
			_ = models.DB.Close()
		}
	})

	cfg := config.NewDefault()
	mgr := session.NewManager()
	r := routers.SetupRouter(cfg, mgr)
	return httptest.NewServer(r)
}

// mustSkipDB 标记当前测试在没有 DB 的情况下跳过（用于不需要 DB 的纯路由/鉴权测试）。
func mustSkipDB(t *testing.T) {
	t.Helper()
	if models.DB == nil {
		t.Skip("DB not available")
	}
}

// login 发送登录请求并返回 YDSZ_SESSION cookie 值。
func login(t *testing.T, serverURL string) string {
	t.Helper()

	// 用 JSON 格式登录
	payload := `{"username":"` + testAdminUser + `","password":"` + testAdminPass + `"}`
	req, err := http.NewRequest(http.MethodPost, serverURL+"/admin/login", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login expected 200, got %d", resp.StatusCode)
	}

	for _, c := range resp.Cookies() {
		if c.Name == "YDSZ_SESSION" {
			return c.Value
		}
	}
	t.Fatal("login did not return YDSZ_SESSION cookie")
	return ""
}

// doAuthed 构造已认证（Cookie + CSRF 头）的 HTTP 请求。
func doAuthed(t *testing.T, serverURL, cookie, method, path string, body interface{}) *http.Request {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, serverURL+path, r)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.AddCookie(&http.Cookie{Name: "YDSZ_SESSION", Value: cookie})
	return req
}

// ==================== 鉴权测试 ====================

func TestAuth_Unauthenticated_ProtectedGET_Returns401(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/client/queryAll")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET /client/queryAll: expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_Unauthenticated_ProtectedPOST_Returns401(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/client/add", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated POST /client/add: expected 401, got %d", resp.StatusCode)
	}
}

func TestAuth_MissingCSRFHeader_POST_Returns403(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	cookie := login(t, srv.URL)

	// 故意不设置 X-Requested-With 头
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/client/add", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "YDSZ_SESSION", Value: cookie})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST without CSRF header: expected 403, got %d", resp.StatusCode)
	}
}

func TestAuth_ValidCSRF_POST_ReturnsOK(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	cookie := login(t, srv.URL)

	req := doAuthed(t, srv.URL, cookie, http.MethodPost, "/client/add", map[string]interface{}{
		"ip":   "10.0.0.99",
		"port": "2020",
		"vkey": "testkey",
	})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("POST with CSRF: expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
}

// ==================== 登录测试 ====================

func TestLogin_WrongPassword_Returns401(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	payload := `{"username":"` + testAdminUser + `","password":"wrong_password"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/login", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password: expected 401, got %d", resp.StatusCode)
	}

	// 即使 session ID cookie 被设置，也不应代表已登录会话。
	// 校验：使用返回的 cookie 访问受保护路由，应仍为 401。
	var gotCookie string
	for _, c := range resp.Cookies() {
		if c.Name == "YDSZ_SESSION" {
			gotCookie = c.Value
			break
		}
	}
	if gotCookie != "" {
		authReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/client/queryAll", nil)
		authReq.AddCookie(&http.Cookie{Name: "YDSZ_SESSION", Value: gotCookie})
		authResp, err := http.DefaultClient.Do(authReq)
		if err != nil {
			t.Fatalf("auth check failed: %v", err)
		}
		defer authResp.Body.Close()
		if authResp.StatusCode != http.StatusUnauthorized {
			t.Errorf("wrong-password session should NOT be authenticated; got %d", authResp.StatusCode)
		}
	}
}

func TestLogin_WrongUsername_Returns401(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	payload := `{"username":"nonexistent_user_xyz","password":"any"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/login", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong username: expected 401, got %d", resp.StatusCode)
	}
}

// ==================== 客户端 CRUD 测试 ====================

func TestClient_Delete_RequiresPOST(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	cookie := login(t, srv.URL)

	// GET 方式的 /client/delete 现在应该 404（router 已改为只接受 POST）
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/client/delete?id=1", nil)
	req.AddCookie(&http.Cookie{Name: "YDSZ_SESSION", Value: cookie})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /client/delete should now 404 (POST only), got %d", resp.StatusCode)
	}
}

func TestClient_ChangeStatus_AcceptsJSONBody(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	cookie := login(t, srv.URL)

	req := doAuthed(t, srv.URL, cookie, http.MethodPost, "/client/changeStatus", map[string]interface{}{
		"id":     999,
		"status": 1,
	})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	// id=999 不存在，但请求应正常处理（返回业务码，影响行数为 0）
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("POST /client/changeStatus: expected 200, got %d, body=%s", resp.StatusCode, string(body))
	}
}