// Package middleware 提供跨切面关注点中间件：traceId 注入、统一响应包装、panic 恢复。
//
// 设计目标（PRD FR-1405）：
//   - 为每个进入的请求注入 traceId（优先从 X-Request-ID 头透传，否则自动生成）
//   - 自动生成符合 W3C TraceContext 规范的 trace ID（16 字节随机十六进制）
//   - panic 恢复返回统一 JSON 格式，不暴露内部堆栈给客户端
package middleware

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// traceIDHeader 透传 traceId 的 HTTP 头名称。
	traceIDHeader  = "X-Request-ID"
	traceIDContext = "traceId"
	traceIDLength  = 16 // 字节
)

// TraceID 注入 traceId 到请求上下文。
//
// 优先级：
//   - 请求头 X-Request-ID 存在且格式合法时直接透传
//   - 否则自动生成 16 字节随机十六进制字符串
//
// 注入后：
//   - gin 上下文 key "traceId" 可由 c.Get("traceId") 获取
//   - 响应头 X-Request-ID 也写入该值（前端可直接从响应头读取）
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(traceIDHeader)
		if !isValidTraceID(traceID) {
			traceID = generateTraceID()
		}
		c.Set(traceIDContext, traceID)
		c.Header(traceIDHeader, traceID)
		c.Next()
	}
}

// RecoveryWithRecovery 自定义 panic 恢复中间件。
//
// 功能：
//   - 捕获 handler 链中的 panic
//   - 返回统一 JSON 格式 500 错误（不暴露内部堆栈）
//   - 关联请求的 traceId，便于日志与响应对应
//   - 记录错误日志供研发排查
func RecoveryWithRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				traceID, _ := c.Get(traceIDContext)
				log.Printf("[panic] traceId=%v path=%s error=%v", traceID, c.Request.URL.Path, r)
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code":    "500",
					"message": "服务端内部错误",
				})
			}
		}()
		c.Next()
	}
}

// GetTraceID 从 gin 上下文提取 traceId。
func GetTraceID(c *gin.Context) string {
	if v, exists := c.Get(traceIDContext); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// generateTraceID 生成 16 字节随机十六进制字符串作为 traceId。
func generateTraceID() string {
	b := make([]byte, traceIDLength)
	if _, err := crand.Read(b); err != nil {
		// crypto/rand 极少失败；回退使用时间戳纳秒
		return fmt.Sprintf("fb%dn%d", time.Now().UnixNano(), b[0])
	}
	return hex.EncodeToString(b)
}

// isValidTraceID 校验 traceId 格式（仅含十六进制字符，长度 8-64）。
func isValidTraceID(id string) bool {
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for _, ch := range id {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}
