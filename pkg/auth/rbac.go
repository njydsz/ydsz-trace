// Package auth 提供轻量 RBAC 模型与中间件生成器。
//
// 不引入新表；角色存储在会话中（登录时下发），权限检查在中间件完成。
//
// 角色层级（高到低）：
//   - Admin：全部权限，含用户/角色管理、系统配置
//   - Operator：管理客户端、日志项、发起查询
//   - Viewer：只读（查询日志、浏览项）
//
// 多租户：当前通过请求会话上下文中的 tenant 字段区分（需在登录/切换时下发），
// 资源行的 tenant_id 列隔离需另行 schema migration；本包提供上下文字段写入/读取 Helpers。
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"ydsz-trace/pkg/session"
)

// Role 角色层级值越大权限越高。
type Role int

const (
	// RoleViewer 只读
	RoleViewer Role = iota
	// RoleOperator 运维（管理资源 + 查询）
	RoleOperator
	// RoleAdmin 全部权限
	RoleAdmin
)

// roleNames Role 的人类可读名。
var roleNames = map[Role]string{
	RoleViewer:   "viewer",
	RoleOperator: "operator",
	RoleAdmin:    "admin",
}

// String 返回角色名。
func (r Role) String() string {
	if s, ok := roleNames[r]; ok {
		return s
	}
	return "unknown"
}

// RoleFromName 从字符串解析角色；未识别返回 RoleViewer。
func RoleFromName(s string) Role {
	switch s {
	case "admin":
		return RoleAdmin
	case "operator":
		return RoleOperator
	case "viewer":
		return RoleViewer
	}
	return RoleViewer
}

// sessionRoleKey / sessionTenantKey 会话字段键名。
const (
	sessionRoleKey   = "role"
	sessionTenantKey = "tenant"
)

// SetRoleToSession 把角色写入会话。
func SetRoleToSession(c *gin.Context, role Role) {
	session.Set(c, sessionRoleKey, role.String())
}

// RoleFromSession 读取会话中的角色；未设置默认 RoleViewer。
func RoleFromSession(c *gin.Context) Role {
	v := session.GetString(c, sessionRoleKey)
	if v == "" {
		return RoleViewer
	}
	return RoleFromName(v)
}

// HasRole 检查会话角色是否满足最低要求。
func HasRole(c *gin.Context, min Role) bool {
	return RoleFromSession(c) >= min
}

// SetTenantToSession 把租户标识写入会话（用于多租户上下文）。
func SetTenantToSession(c *gin.Context, tenant string) {
	if tenant == "" {
		tenant = "default"
	}
	session.Set(c, sessionTenantKey, tenant)
}

// TenantFromSession 读取会话中的租户标识。
func TenantFromSession(c *gin.Context) string {
	t := session.GetString(c, sessionTenantKey)
	if t == "" {
		return "default"
	}
	return t
}

// RequireRole 返回 Gin 中间件：要求会话角色不低于 min。
//
// 用法：
//
//	r.GET("/admin/users", auth.RequireRole(auth.RoleAdmin), admin.ListUsers)
func RequireRole(min Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !HasRole(c, min) {
			c.JSON(http.StatusForbidden, gin.H{
				"code": "403",
				"msg":  "权限不足，需要 " + min.String() + " 或更高角色",
				"data": nil,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAdmin = RequireRole(RoleAdmin)。
func RequireAdmin() gin.HandlerFunc { return RequireRole(RoleAdmin) }

// RequireOperator = RequireRole(RoleOperator)。
func RequireOperator() gin.HandlerFunc { return RequireRole(RoleOperator) }
