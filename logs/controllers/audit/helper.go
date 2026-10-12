// Package audit 提供审计日志的记录与查询控制器。
//
// RecordAudit 为包级辅助函数，供各业务 handler 调用，写入失败仅打日志，不影响主流程。
package audit

import (
	"log"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// RecordAudit 记录一条审计日志。
//
// 参数：
//   - c: gin 上下文（用于取 username 与 clientIP）
//   - action: 操作类型（search / export / login / config_change / delete 等）
//   - resource: 操作对象（/logs/search, /client/register 等）
//   - detail: 详情 map，会被序列化为 JSON 字符串存储
//   - result: success / fail
//
// 写入失败仅打日志，不返回错误，不影响主业务流。
func RecordAudit(c *gin.Context, action, resource string, detail map[string]interface{}, result string) {
	username := session.GetString(c, "username")
	clientIP := c.ClientIP()

	if err := models.AppendAuditLog(username, action, resource, detail, clientIP, result); err != nil {
		log.Printf("record audit log failed: action=%s resource=%s user=%s err=%v",
			action, resource, username, err)
	}
}
