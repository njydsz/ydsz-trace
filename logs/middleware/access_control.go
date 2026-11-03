// Package middleware 提供 Gin 中间件：行级 RBAC 访问控制。
//
// V2.1 FR-6001：多用户行级数据权限。
//
// 中间件职责：
//   - 跳过公开路径（登录、健康检查、静态资源等）
//   - admin 角色跳过行级校验
//   - operator/viewer 查询 t_user_binding 表获取可访问资源 ID 列表，注入 gin.Context
//   - 无任何绑定的非 admin 用户直接拒绝（403）
//
// 注入的上下文 key：
//   - "accessible_items":   []int64 — 用户可访问的 t_item 行 ID 列表
//   - "accessible_clients": []int64 — 用户可访问的 t_client 行 ID 列表
//   - "bound_user_id":      int64   — 当前用户的 DB ID
//
// 使用方式（router.go 后续接入）：
//
//	r.Use(access_control.AccessControlMiddleware())
package middleware

import (
	"strconv"
	"strings"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	pkgauth "ydsz-trace/pkg/auth"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// 跳过行级校验的路径前缀（公开路径/静态资源/登录接口）。
var skipPathPrefixes = []string{
	"/admin/login",
	"/admin/exit",
	"/static/",
	"/health",
	"/ready",
	"/metrics",
}

// 跳过行级校验的精确路径（client/register 等 logc 代理回调）。
var skipPathExact = map[string]bool{
	"/":                true,
	"/client/register": true,
	"/admin/console":   true,
}

// AccessControlMiddleware 返回 Gin 中间件，实施行级 RBAC 过滤。
//
// 处理逻辑：
//  1. 公开路径/精确匹配路径 → 直接放行
//  2. admin 角色 → 跳过行级校验（仍由 authBlockMiddleware 保证已登录）
//  3. operator/viewer → 由 session 中的 username 查找 DB 获取 user_id，
//     查询 t_user_binding 获取可访问资源 ID 列表注入 context；无绑定则 403
//
// 若用户通过 env/config 回退登录（不在 t_user 表中），视为兼容用户直接放行。
func AccessControlMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 规则 1：公开路径直接放行
		if isSkippedPath(path) {
			c.Next()
			return
		}

		// 规则 2：admin 跳过行级校验
		role := pkgauth.RoleFromSession(c)
		if role == pkgauth.RoleAdmin {
			c.Next()
			return
		}

		// 规则 3：operator / viewer — 查 DB 取 user_id
		username := session.GetString(c, "username")
		if username == "" {
			api.Fail(c, api.CodeForbidden, models.ErrForbiddenError.Error())
			c.Abort()
			return
		}

		user, err := models.GetUserByUsername(username)
		if err != nil || user.ID == 0 {
			// env/config 回退用户（不在 t_user 表中）→ 兼容放行
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// 获取可访问的 item 列表
		itemIDs, err := models.GetUserResourceIDs(ctx, user.ID, models.ResourceItem)
		if err != nil {
			api.Fail(c, api.CodeServerError, "查询用户资源绑定失败")
			c.Abort()
			return
		}

		// 获取可访问的 client 列表
		clientIDs, err := models.GetUserResourceIDs(ctx, user.ID, models.ResourceClient)
		if err != nil {
			api.Fail(c, api.CodeServerError, "查询用户资源绑定失败")
			c.Abort()
			return
		}

		// 规则 4：无任何绑定记录的非 admin 用户 → 403
		if len(itemIDs) == 0 && len(clientIDs) == 0 {
			api.Fail(c, api.CodeForbidden, models.ErrForbiddenError.Error())
			c.Abort()
			return
		}

		// 注入上下文，供下游 handler / filter 使用
		c.Set("accessible_items", itemIDs)
		c.Set("accessible_clients", clientIDs)
		c.Set("bound_user_id", user.ID)

		c.Next()
	}
}

// isSkippedPath 判断路径是否应跳过行级 RBAC 校验。
func isSkippedPath(path string) bool {
	if skipPathExact[path] {
		return true
	}
	for _, prefix := range skipPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// ItemIDsFromContext 从 gin 上下文中提取可访问 item ID 列表。
// 返回 nil, false 表示上下文不存在该 key（admin 跳过场景或中间件未注入）。
func ItemIDsFromContext(c *gin.Context) ([]int64, bool) {
	v, ok := c.Get("accessible_items")
	if !ok {
		return nil, false
	}
	ids, ok := v.([]int64)
	return ids, ok
}

// ClientIDsFromContext 从 gin 上下文中提取可访问 client ID 列表。
// 返回 nil, false 表示上下文不存在该 key（admin 跳过场景或中间件未注入）。
func ClientIDsFromContext(c *gin.Context) ([]int64, bool) {
	v, ok := c.Get("accessible_clients")
	if !ok {
		return nil, false
	}
	ids, ok := v.([]int64)
	return ids, ok
}

// UserIDFromContext 从 gin 上下文中提取已绑定的用户 ID。
// 返回 0, false 表示上下文不存在该 key（admin 跳过场景）。
func UserIDFromContext(c *gin.Context) (int64, bool) {
	v, ok := c.Get("bound_user_id")
	if !ok {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok
}

// ─── Handler 内联校验（V2.1-04 接线） ───
//
// 下列函数供 controller 在读取资源前调用，统一完成「判定 + 写 403」两步，
// 避免每个 handler 重复 if 判断与响应封装。
//
// 设计约束：
//   - 上下文无 accessible_* → admin 或中间件未启用 → 一律放行
//   - 拒绝时写入统一 403 响应并返回 false，handler 直接 return

// CheckItem 校验 item 资源访问权；拒绝时写 403 并返回 false。
func CheckItem(c *gin.Context, itemID int64) bool {
	ids, ok := ItemIDsFromContext(c)
	if !ok {
		return true
	}
	if containsInt64(ids, itemID) {
		return true
	}
	api.Fail(c, api.CodeForbidden, models.ErrForbiddenError.Error())
	return false
}

// CheckClient 校验 client 资源访问权；拒绝时写 403 并返回 false。
func CheckClient(c *gin.Context, clientID int64) bool {
	ids, ok := ClientIDsFromContext(c)
	if !ok {
		return true
	}
	if containsInt64(ids, clientID) {
		return true
	}
	api.Fail(c, api.CodeForbidden, models.ErrForbiddenError.Error())
	return false
}

// CheckResourceID 从请求中提取资源 ID 并校验 item 访问权。
//
// 适用于 query/path 携带 id 的通用场景；无有效 ID 时放行。
func CheckResourceID(c *gin.Context) bool {
	id := resourceIDFromRequest(c)
	if id <= 0 {
		return true
	}
	return CheckItem(c, id)
}

// resourceIDFromRequest 从 query "id" 或 path "id" 提取资源 ID。
func resourceIDFromRequest(c *gin.Context) int64 {
	if id, err := strconv.ParseInt(c.Query("id"), 10, 64); err == nil && id > 0 {
		return id
	}
	if id, err := strconv.ParseInt(c.Param("id"), 10, 64); err == nil && id > 0 {
		return id
	}
	return 0
}

// containsInt64 判断切片是否包含目标值。
func containsInt64(s []int64, v int64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// FilterItemHandler 包装 gin.HandlerFunc，对请求中的 item ID 做行级校验。
//
// 列表类接口（无具体 ID）自动放行；单资源接口命中即校验。
func FilterItemHandler(h gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !CheckResourceID(c) {
			return
		}
		h(c)
	}
}

// FilterClientHandler 与 FilterItemHandler 同构，校验 client 资源。
func FilterClientHandler(h gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := resourceIDFromRequest(c)
		if id > 0 && !CheckClient(c, id) {
			return
		}
		h(c)
	}
}
