// Package controllers_test 包含 NL2DSL HTTP 端到端集成测试。
//
// 验证 /logs/nl2dsl/parse 端点的请求响应链路。
package controllers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestNl2DslHTTP_Parse 验证 NL2DSL HTTP 端点的完整请求链路。
func TestNl2DslHTTP_Parse(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	cookie := login(t, srv.URL)

	body := `{"query":"近 1 小时 ERROR 日志"}`
	req := doAuthed(t, srv.URL, cookie, http.MethodPost, "/logs/nl2dsl/parse", nil)
	req.Body = io.NopCloser(strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("NL2DSL parse: 期望 200, 得到 %d, body=%s", resp.StatusCode, string(b))
	}

	rawBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		t.Fatalf("decode response failed: %v, body=%s", err, string(rawBody))
	}

	// 验证响应结构
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("响应缺少 data 字段: %v", result)
	}

	if data["level"] != "ERROR" {
		t.Errorf("期望 level=ERROR, 得到=%v", data["level"])
	}
	if data["userIntent"] != "近 1 小时 ERROR 日志" {
		t.Errorf("userIntent 应等于原始查询, 得到=%v", data["userIntent"])
	}
	if data["dslConditions"] == nil || data["dslConditions"] == "" {
		t.Errorf("dslConditions 不应为空, 得到=%v", data["dslConditions"])
	}
}

// TestNl2DslHTTP_Unauthenticated 验证未登录调用 NL2DSL 端点返回 401。
func TestNl2DslHTTP_Unauthenticated(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	body := `{"query":"ERROR"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/logs/nl2dsl/parse", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("未登录 NL2DSL: 期望 401, 得到 %d", resp.StatusCode)
	}
}

// TestNl2DslHTTP_EmptyQuery 验证空查询返回 400。
func TestNl2DslHTTP_EmptyQuery(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	cookie := login(t, srv.URL)

	body := `{"query":""}`
	req := doAuthed(t, srv.URL, cookie, http.MethodPost, "/logs/nl2dsl/parse", nil)
	req.Body = io.NopCloser(strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("空查询: 期望 400, 得到 %d", resp.StatusCode)
	}
}

// TestNl2DslHTTP_Keyword 验证关键词提取的端到端链路。
func TestNl2DslHTTP_Keyword(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	cookie := login(t, srv.URL)

	body := `{"query":"查找 user_login 相关日志"}`
	req := doAuthed(t, srv.URL, cookie, http.MethodPost, "/logs/nl2dsl/parse", nil)
	req.Body = io.NopCloser(strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("NL2DSL parse: 期望 200, 得到 %d, body=%s", resp.StatusCode, string(b))
	}

	var result map[string]interface{}
	rawBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(rawBody, &result); err != nil {
		t.Fatalf("decode failed: %v, body=%s", err, string(rawBody))
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("缺少 data 字段")
	}
	if data["level"] != "" {
		t.Errorf("无级别查询: level 应为空, 得到=%v", data["level"])
	}
}
