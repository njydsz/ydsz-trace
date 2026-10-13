// Package middleware 单元测试。
//
// 覆盖 TraceID 中间件的以下场景：
//   - 自动生成合法 traceId
//   - 从 X-Request-ID 头透传合法 traceId
//   - 拒绝非法格式的 X-Request-ID 并生成新 traceId
//   - 响应头中包含 X-Request-ID
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTraceID_GeneratesValidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())
	r.GET("/test", func(c *gin.Context) {
		traceID := GetTraceID(c)
		assert.NotEmpty(t, traceID, "traceId 不应为空")
		assert.True(t, isValidTraceID(traceID), "traceId 格式应合法")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	// 响应头应包含 traceId
	respHeader := w.Header().Get("X-Request-ID")
	assert.NotEmpty(t, respHeader, "响应头 X-Request-ID 不应为空")
	assert.True(t, isValidTraceID(respHeader), "响应头 traceId 格式应合法")
}

func TestTraceID_TransfersValidHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())
	r.GET("/test", func(c *gin.Context) {
		traceID := GetTraceID(c)
		assert.Equal(t, "abcdef1234567890", traceID, "应透传请求头中的 traceId")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "abcdef1234567890")
	r.ServeHTTP(w, req)
}

func TestTraceID_RejectsInvalidHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())
	r.GET("/test", func(c *gin.Context) {
		traceID := GetTraceID(c)
		// 非法格式的 traceId 应被替换
		assert.NotEqual(t, "invalid id with spaces!", traceID)
		assert.True(t, isValidTraceID(traceID), "应生成合法 traceId")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "invalid id with spaces!")
	r.ServeHTTP(w, req)
}

func TestTraceID_Uniqueness(t *testing.T) {
	// 验证连续生成的 traceId 不重复
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateTraceID()
		assert.False(t, ids[id], "traceId 应唯一，但 %s 重复", id)
		ids[id] = true
	}
}

func TestIsValidTraceID_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"空字符串", "", false},
		{"过短", "abc", false},
		{"合法短ID", "abcdef12", true},
		{"合法长ID", "abcdef1234567890abcdef1234567890", true},
		{"含非法字符", "abcdefg!", false},
		{"含空格", "abcdef 12", false},
		{"边界长度7", "abcdef1", false},
		{"边界长度8", "abcdef12", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidTraceID(tt.id))
		})
	}
}
