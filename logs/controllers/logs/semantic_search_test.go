// Package logs 语义检索控制器单元测试。
//
// 验证参数校验逻辑：空 query、非法 type、TopK 默认值。
package logs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSemanticSearch_EmptyQuery 验证空 query 返回 400。
func TestSemanticSearch_EmptyQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(SemanticSearchReq{Query: "", Type: SemanticTargetTemplate})
	c.Request, _ = http.NewRequest(http.MethodPost, "/logs/semantic/search", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	SemanticSearch(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty query, got %d, body=%s", w.Code, w.Body.String())
	}
}

// TestSemanticSearch_InvalidType 验证非法 type 返回 400。
func TestSemanticSearch_InvalidType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(SemanticSearchReq{Query: "数据库连接失败", Type: "invalid"})
	c.Request, _ = http.NewRequest(http.MethodPost, "/logs/semantic/search", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	SemanticSearch(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid type, got %d, body=%s", w.Code, w.Body.String())
	}
}

// TestSemanticSearch_NoTopK 验证未传 TopK 时默认 10。
//
// 参数校验通过后，由于测试环境中无 ollama，会触发降级响应（500+降级提示）。
// 此处重点验证：未传 TopK 不会触发 400 参数错误，即默认值生效。
func TestSemanticSearch_NoTopK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, _ := json.Marshal(map[string]string{"query": "超时异常", "type": "template"})
	c.Request, _ = http.NewRequest(http.MethodPost, "/logs/semantic/search", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	SemanticSearch(c)

	// 不应为 400：query 非空、type 合法，TopK 默认值生效。
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected non-400 (TopK default accepted), got 400, body=%s", w.Body.String())
	}

	// 无 ollama 服务时应触发降级（500 或 504 等），包含降级提示文本。
	respBody := w.Body.String()
	if !contains(respBody, "语义检索暂不可用") {
		t.Logf("注意：响应中未包含降级提示，可能是 ollama 可用环境: %s", respBody)
	}
}

// contains 简易子串判断（避免 strings 导入冗余）。
func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
