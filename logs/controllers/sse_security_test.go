// Package controllers_test 包含 SSE 接口安全封装测试（P0-4）。
//
// 验证 LogTail/Stream SSE 接口的鉴权与 CSRF 防护：
//   - 未登录请求返回 401
//   - 缺少 CSRF 防护头返回 403
//   - 已登录用户正常通过鉴权
package controllers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestSSE_Tail_Unauthenticated 验证未登录访问 /logs/tail 返回 401。
func TestSSE_Tail_Unauthenticated(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_ADMIN_USER", "admin")
	t.Setenv("YDSZ_ADMIN_PASSWORD", "admin123")

	// 未登录 POST → 403 (CSRF 优先) 或 401 (session)
	body := `{"client":1,"item":1,"date":"20260101"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/logs/tail", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// 注意：不添加 X-Requested-With → 触发 CSRF 403

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("未登录+无CSRF头: 期望 403, 得到 %d, body=%s", resp.StatusCode, string(b))
	}
}

// TestSSE_Tail_CSFR_Passes_Session_401 验证携带 CSRF 头但未登录返回 401。
func TestSSE_Tail_CSFR_Passes_Session_401(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_ADMIN_USER", "admin")
	t.Setenv("YDSZ_ADMIN_PASSWORD", "admin123")

	body := `{"client":1,"item":1,"date":"20260101"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/logs/tail", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("CSRF通过+未登录: 期望 401, 得到 %d, body=%s", resp.StatusCode, string(b))
	}
}

// TestSSE_Stream_Unauthenticated 验证未登录访问 /logs/stream 返回 401/403。
func TestSSE_Stream_Unauthenticated(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_ADMIN_USER", "admin")
	t.Setenv("YDSZ_ADMIN_PASSWORD", "admin123")

	body := `{"taskNo":"T001"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/logs/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("未登录+CSRF头: 期望 401, 得到 %d, body=%s", resp.StatusCode, string(b))
	}
}

// TestSSE_Tail_Authenticated 验证已登录用户 SSE 鉴权通过。
func TestSSE_Tail_Authenticated(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_ADMIN_USER", "admin")
	t.Setenv("YDSZ_ADMIN_PASSWORD", "admin123")

	// 1. 登录获取 session cookie
	loginBody := `{"username":"admin","password":"admin123"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: 期望 200, 得到 %d", resp.StatusCode)
	}

	// 提取 cookie
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" || c.Name == "sid" || strings.Contains(c.Name, "session") {
			sessionCookie = c
			break
		}
	}

	// 2. 携带 session cookie 访问 SSE 接口
	body := `{"client":1,"item":1,"date":"20260101"}`
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/logs/tail", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Requested-With", "XMLHttpRequest")
	req2.Header.Set("Accept", "text/event-stream")
	if sessionCookie != nil {
		req2.AddCookie(sessionCookie)
	}
	// 添加其他可能的 cookie
	for _, c := range cookies {
		req2.AddCookie(c)
	}

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("tail request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp2.Body); resp2.Body.Close() }()

	// SSE 成功建立：200 + text/event-stream content-type（或者前端 redirect 提示）
	if resp2.StatusCode == http.StatusUnauthorized || resp2.StatusCode == http.StatusForbidden {
		b, _ := io.ReadAll(resp2.Body)
		var apiResp map[string]interface{}
		if err := json.Unmarshal(b, &apiResp); err == nil {
			t.Fatalf("已登录 SSE 应通过鉴权: 得到 %d, msg=%v", resp2.StatusCode, apiResp["msg"])
		}
		t.Fatalf("已登录 SSE 应通过鉴权: 得到 %d", resp2.StatusCode)
	}
}

// TestSSE_CSRF_Token_ValidWithSession 验证携带 CSRF 头但未登录的 POST 返回 401（不是 403）。
func TestSSE_CSRF_WithSession_Check401(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// 不带 session cookie，只带 X-Requested-With
	body := `{"client":1,"item":1,"date":"20260101"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/logs/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	code := resp.StatusCode
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if code != http.StatusUnauthorized {
		t.Errorf("无session+CSRF: 期望 401, 得到 %d", code)
	}
}
