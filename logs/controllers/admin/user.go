// Package admin 包含用户认证、SPA 控制台与多用户管理控制器。
//
// 安全说明：
//   - 账号密码来自环境变量 / 配置文件（当前为单账号，后续可扩展）
//   - 密码使用加盐哈希比对（pkg/auth 模块），支持明文→哈希平滑迁移
//   - 登录接口内置单 IP 速率限制，防御暴力破解
//   - 用户管理（CRUD）通过 t_user 表支持多账号
package admin

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/logs/controllers/audit"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/auth"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/ratelimit"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// ========== 登录速率限制 ==========

// 默认速率限制器在 pkg/ratelimit 包中初始化（进程内存版）。
// 多副本部署时，应在 main 中调用 ratelimit.SetDefault 切换为 Redis 版本。

// extractClientIP 从 gin 上下文中提取客户端真实 IP。
func extractClientIP(c *gin.Context) string {
	// 优先从 X-Forwarded-For 取（反向代理场景）
	if xff := c.Request.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	if xrip := c.Request.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	// 直连场景
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}

// User 用户登录凭证（请求体）。
type User struct {
	// Username 用户名
	Username string `json:"username"`
	// Password 密码（明文传输，需 HTTPS）
	Password string `json:"password"`
}

// HealthResp 健康检查响应结构。
type HealthResp struct {
	// Status 状态标识
	Status string `json:"status"`
	// App 服务标识
	App string `json:"app"`
	// Time 当前服务器本地时间
	Time string `json:"time"`
}

// Health 存活探针（K8s liveness probe）。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ok",
		App:    "ydsz-trace-logs",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// Ready 就绪探针（K8s readiness probe）。
func Ready(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ready",
		App:    "ydsz-trace-logs",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// getAdminUser 获取管理员用户名（环境变量 YDSZ_ADMIN_USER > 配置文件）。
func getAdminUser(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_ADMIN_USER"); v != "" {
		return v
	}
	return cfg.StringOr("username", "admin")
}

// getAdminPassword 获取管理员密码（环境变量 YDSZ_ADMIN_PASSWORD > 配置文件）。
//
// 安全提示：生产必须使用环境变量注入强密码，避免写在配置文件中。
func getAdminPassword(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_ADMIN_PASSWORD"); v != "" {
		return v
	}
	return cfg.StringOr("password", "change_me_production")
}

// logvRoot 前端构建产物根目录（环境变量 YDSZ_LOGV_ROOT > 默认 logv/dist）。
func logvRoot() string {
	if v := os.Getenv("YDSZ_LOGV_ROOT"); v != "" {
		return v
	}
	return "logv/dist"
}

// Index 控制台 SPA 入口，返回 index.html。
func Index(c *gin.Context) {
	serveIndex(c)
}

// Console 兼容旧路由（重定向到 index.html）。
func Console(c *gin.Context) {
	serveIndex(c)
}

// serveIndex 返回 Vite 构建产物 index.html；未构建时返回部署指引文本。
func serveIndex(c *gin.Context) {
	root := logvRoot()
	indexFile := filepath.Join(root, "index.html")
	if _, err := os.Stat(indexFile); err != nil {
		c.String(http.StatusOK,
			"Ydsz Trace console. 前端尚未构建：请在 logv/ 目录执行 `npm install && npm run build`，"+
				"并将产物 logv/dist 置于可访问路径（环境变量 YDSZ_LOGV_ROOT 可指定）。")
		return
	}
	c.File(indexFile)
}

// ServeStatic 处理静态资源与 SPA history 回退：
//
//   - API 前缀未命中返回 404（避免误当 SPA 页面）
//   - 文件存在直接返回，否则回退到 index.html
func ServeStatic(c *gin.Context) {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		api.Fail(c, api.CodeNotFound, "not found")
		return
	}

	reqPath := c.Request.URL.Path
	// API 前缀未匹配到具体路由时，直接返回 404，避免把 API 404 误当 SPA 页面
	apiPrefixes := []string{"/admin", "/client", "/item", "/logs", "/health", "/ready"}
	for _, p := range apiPrefixes {
		if strings.HasPrefix(reqPath, p) {
			api.Fail(c, api.CodeNotFound, "not found")
			return
		}
	}

	root := logvRoot()
	if _, err := os.Stat(root); err != nil {
		api.Fail(c, api.CodeNotFound, "frontend not found")
		return
	}

	clean := filepath.Clean("/" + reqPath)
	filePath := filepath.Join(root, clean)
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		c.File(filePath)
		return
	}
	// SPA history 路由回退到 index.html
	c.File(filepath.Join(root, "index.html"))
}

// Login 用户登录：校验账号密码并在会话中设置 username。
//
// GET 请求：返回 200，用于前端 SPA 登录页面渲染或探测登录状态（在会话中已有 username 时返回用户名）。
// POST 请求：验证账号密码，成功则设置会话并返回用户名。
//
// 登录策略：
//   - 优先查询 t_user 数据库表（多用户管理）
//   - 数据库无匹配时回退到环境变量/配置文件（兼容老版本单账号）
//
// 安全特性：
//   - 密码加盐哈希比对（pkg/auth 模块）
//   - 禁用状态用户（status=0）拒绝登录
//   - 单 IP 速率限制，防御暴力破解（默认 60 秒内最多 5 次失败，超出封禁 300 秒）
func Login(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	// GET /admin/login：已登录则返回用户名，未登录也返回 200 供前端渲染
	if c.Request.Method == http.MethodGet {
		if userName := session.GetString(c, "username"); userName != "" {
			role := session.GetString(c, "role")
			api.Success(c, "用户已登录", gin.H{"username": userName, "role": role})
			return
		}
		api.Success(c, "未登录", nil)
		return
	}

	// 先获取 session，判断用户是否已经登录
	userName := session.GetString(c, "username")
	if userName != "" {
		role := session.GetString(c, "role")
		api.Success(c, "用户已登录", gin.H{"username": userName, "role": role})
		return
	}

	// 速率限制检查
	clientIP := extractClientIP(c)
	allowed, remainSec := ratelimit.Default.Allow(clientIP)
	if !allowed {
		api.Error(c, http.StatusTooManyRequests, fmt.Sprintf("登录尝试过于频繁，请 %d 秒后再试", remainSec))
		return
	}

	// 用户没有登录，获取请求参数
	var user User
	data, err := c.GetRawData()
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		ratelimit.Default.RecordFailure(clientIP)
		return
	}
	err = json.Unmarshal(data, &user)
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		ratelimit.Default.RecordFailure(clientIP)
		return
	}

	// 1. 优先查询 t_user 数据库（多用户登录）
	if dbUser, dbErr := models.GetUserByUsername(user.Username); dbErr == nil && dbUser.ID > 0 {
		// 检查用户状态
		if dbUser.Status == models.StatusDisabled {
			ratelimit.Default.RecordFailure(clientIP)
			api.Fail(c, api.CodeForbidden, "账号已被禁用，请联系管理员")
			return
		}
		// 校验密码（数据库存储的 argon2id 哈希）
		valid, verifyErr := auth.VerifyPassword(user.Password, dbUser.PasswordHash)
		if verifyErr != nil || !valid {
			ratelimit.Default.RecordFailure(clientIP)
			api.Fail(c, api.CodeUnauthorized, "用户名或密码错误")
			return
		}
		// 登录成功
		ratelimit.Default.RecordSuccess(clientIP)
		session.Set(c, "username", dbUser.Username)
		auth.SetRoleToSession(c, auth.RoleFromName(dbUser.Role))
		auth.SetTenantToSession(c, defaultTenant(cfg))
		api.Success(c, "登录成功", gin.H{"username": dbUser.Username, "role": dbUser.Role})
		return
	}

	// 2. 数据库无匹配 → 回退到环境变量/配置文件（兼容老版本单账号）
	uname := getAdminUser(cfg)
	upwd := getAdminPassword(cfg)

	if uname != user.Username {
		ratelimit.Default.RecordFailure(clientIP)
		api.Fail(c, api.CodeUnauthorized, "用户名或密码错误")
		return
	}

	valid, err := verifyPasswordFallBack(user.Password, upwd)
	if err != nil || !valid {
		ratelimit.Default.RecordFailure(clientIP)
		api.Fail(c, api.CodeUnauthorized, "用户名或密码错误")
		return
	}

	// 登录成功
	ratelimit.Default.RecordSuccess(clientIP)

	// 平滑迁移：明文/旧哈希 → 记录提示
	if auth.NeedsRehash(upwd) {
		if hashed, err := auth.HashPassword(user.Password); err == nil {
			log.Printf("[security] 用户 %s 使用 SHA-256 哈希登录成功，已生成 Argon2id 哈希：%s（请更新至配置/环境变量）", uname, hashed)
		}
	}
	if !auth.IsHashedPassword(upwd) {
		if hashed, err := auth.HashPassword(user.Password); err == nil {
			log.Printf("[security] 用户 %s 使用明文密码登录成功，已生成哈希：%s（请更新至配置）", uname, hashed)
		}
	}

	role := determineRoleForUser(uname, cfg)
	auth.SetRoleToSession(c, role)
	auth.SetTenantToSession(c, defaultTenant(cfg))
	session.Set(c, "username", user.Username)
	api.Success(c, "登录成功", gin.H{"username": uname, "role": role.String()})
}

// determineRoleForUser 根据配置返回用户角色；单管理员部署默认 admin，
// 多账号场景可通过配置文件 / 数据库扩展映射。
func determineRoleForUser(uname string, cfg *config.Config) auth.Role {
	// 默认 admin 账号持 admin 角色
	admin := getAdminUser(cfg)
	if uname == admin {
		return auth.RoleAdmin
	}
	// 非 admin 账号默认 operator / 可由配置扩展
	return auth.RoleOperator
}

// defaultTenant 返回默认租户标识（预留多租户扩展口）。
func defaultTenant(_ *config.Config) string {
	return "default"
}

// verifyPasswordFallBack 凭据回退验证（仅用于 env/config 兼容路径）。
//
// 如果存储的是哈希值，使用 auth.VerifyPassword 校验；
// 如果存储的是明文（兼容旧配置），直接比对并标记需迁移。
func verifyPasswordFallBack(input, stored string) (bool, error) {
	if auth.IsHashedPassword(stored) {
		return auth.VerifyPassword(input, stored)
	}
	// 明文模式：恒定时间比较
	return subtle.ConstantTimeCompare([]byte(input), []byte(stored)) == 1, nil
}

// Exit 退出登录：清除会话中的 username 并销毁会话。
func Exit(c *gin.Context) {
	username := session.GetString(c, "username")
	session.Delete(c, "username")
	session.Destroy(c)
	// 审计埋点：登出
	audit.RecordAudit(c, "logout", "/admin/exit", map[string]interface{}{
		"username": username,
	}, "success")
	api.Success(c, "退出成功", nil)
}

// ========== 多用户管理 CRUD ==========

// userCreateReq 创建用户请求体。
type userCreateReq struct {
	// Username 用户名（唯一）
	Username string `json:"username" binding:"required"`
	// Password 明文密码（服务端用 argon2id 哈希后存储）
	Password string `json:"password" binding:"required,min=6"`
	// Role 用户角色：admin / operator / viewer（默认 viewer）
	Role string `json:"role"`
}

// userUpdateReq 更新用户请求体。
type userUpdateReq struct {
	// Role 用户角色
	Role string `json:"role"`
	// Status 用户状态：1-启用 0-禁用
	Status int `json:"status"`
	// Password 新密码（可选，为空则不改密码）
	Password string `json:"password"`
}

// ListUsers 分页查询用户列表。
//
// @Summary 查询用户列表
// @Description 分页查询系统用户列表，支持关键词搜索。
// @Tags Admin
// @Accept json
// @Produce json
// @Param page query int false "页码（默认 1）"
// @Param limit query int false "每页条数（默认 20）"
// @Param keyword query string false "用户名关键词搜索"
// @Success 200 {object} api.Paginated "用户分页列表"
// @Failure 401 {object} api.Response "未登录"
// @Router /admin/users [get]
func ListUsers(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	keyword := c.Query("keyword")
	page := models.QueryUsers(pageNum, pageSize, keyword)
	api.Paginated(c, "查询用户成功", page.List, page.TotalCount, pageNum, pageSize)
}

// CreateUser 创建用户（密码使用 argon2id 哈希后存储）。
//
// @Summary 创建用户
// @Description 创建新用户，密码将使用 argon2id 哈希后存储。角色可选 admin/operator/viewer。
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body userCreateReq true "用户信息：username + password（必填，>=6 位），role 可选（默认 viewer）"
// @Success 200 {object} api.Response{data=map[string]int64} "创建成功，返回用户 ID"
// @Failure 400 {object} api.Response "参数错误或用户名已存在"
// @Failure 401 {object} api.Response "未登录"
// @Router /admin/users [post]
func CreateUser(c *gin.Context) {
	var req userCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: 用户名和密码不能为空，密码至少 6 位")
		return
	}

	// 校验角色合法性
	role := req.Role
	if role == "" {
		role = "viewer"
	}
	if role != auth.RoleAdmin.String() && role != auth.RoleOperator.String() && role != auth.RoleViewer.String() {
		api.Fail(c, api.CodeBadRequest, "无效的用户角色，允许值: admin / operator / viewer")
		return
	}

	// 校验用户名唯一性
	if models.UserExists(req.Username, 0) {
		api.Fail(c, api.CodeBadRequest, "用户名已存在")
		return
	}

	// 密码 argon2id 哈希
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		api.Fail(c, api.CodeServerError, "密码哈希失败")
		return
	}

	user := &models.TUser{
		Username:     req.Username,
		PasswordHash: passwordHash,
		Role:         role,
		Status:       models.StatusEnabled,
	}
	id, err := models.AddUser(user)
	if err != nil {
		api.Fail(c, api.CodeServerError, "创建用户失败")
		return
	}
	api.Success(c, "创建用户成功", gin.H{"id": id})
}

// UpdateUser 更新用户信息（角色、状态、密码）。
//
// @Summary 更新用户
// @Description 更新用户的角色、状态（启用/禁用）或密码。
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param body body userUpdateReq true "更新字段：role / status / password（均为可选）"
// @Success 200 {object} api.Response "更新成功"
// @Failure 400 {object} api.Response "参数错误或无效角色"
// @Failure 401 {object} api.Response "未登录"
// @Failure 404 {object} api.Response "用户不存在"
// @Router /admin/users/{id} [put]
func UpdateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "无效的用户 ID")
		return
	}

	var req userUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}

	// 查询用户是否存在
	existing, err := models.GetUserByID(id)
	if err != nil {
		api.Fail(c, api.CodeNotFound, "用户不存在")
		return
	}

	// 校验角色合法性
	if req.Role != "" {
		if req.Role != auth.RoleAdmin.String() && req.Role != auth.RoleOperator.String() && req.Role != auth.RoleViewer.String() {
			api.Fail(c, api.CodeBadRequest, "无效的用户角色，允许值: admin / operator / viewer")
			return
		}
		existing.Role = req.Role
	}

	// 更新状态（Status 零值视为未传，Go 无法区分 0 与未传，约定 status 字段只有显式传 0 或 1 才更新）
	existing.Status = req.Status

	// 更新密码（如传入新密码）
	if req.Password != "" {
		if len(req.Password) < 6 {
			api.Fail(c, api.CodeBadRequest, "密码至少 6 位")
			return
		}
		passwordHash, err := auth.HashPassword(req.Password)
		if err != nil {
			api.Fail(c, api.CodeServerError, "密码哈希失败")
			return
		}
		if err := models.UpdateUserPassword(id, passwordHash); err != nil {
			api.Fail(c, api.CodeServerError, "更新密码失败")
			return
		}
	}

	if err := models.UpdateUser(&existing); err != nil {
		api.Fail(c, api.CodeServerError, "更新用户失败")
		return
	}
	api.Success(c, "更新用户成功", nil)
}

// changePasswordReq 自助修改密码请求体。
type changePasswordReq struct {
	// OldPassword 旧密码（明文）
	OldPassword string `json:"oldPassword" binding:"required"`
	// NewPassword 新明文密码（>= 6 位）
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// ChangePassword 自助修改当前登录用户密码。
//
// @Summary 修改自己的密码
// @Description 已登录用户修改自身密码，需验证旧密码。
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body changePasswordReq true "oldPassword + newPassword"
// @Success 200 {object} api.Response "修改成功"
// @Failure 400 {object} api.Response "参数错误或旧密码不正确"
// @Failure 401 {object} api.Response "未登录"
// @Router /admin/users/change-password [post]
func ChangePassword(c *gin.Context) {
	username := session.GetString(c, "username")
	if username == "" {
		api.Fail(c, api.CodeUnauthorized, "未登录")
		return
	}

	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: 旧密码和新密码不能为空，新密码至少 6 位")
		return
	}

	// 查询当前用户
	dbUser, err := models.GetUserByUsername(username)
	if err != nil {
		api.Fail(c, api.CodeNotFound, "用户不存在")
		return
	}

	// 验证旧密码
	valid, verifyErr := auth.VerifyPassword(req.OldPassword, dbUser.PasswordHash)
	if verifyErr != nil || !valid {
		api.Fail(c, api.CodeBadRequest, "旧密码不正确")
		return
	}

	// 新旧密码不能相同
	if req.OldPassword == req.NewPassword {
		api.Fail(c, api.CodeBadRequest, "新密码不能与旧密码相同")
		return
	}

	// 哈希新密码并更新
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		api.Fail(c, api.CodeServerError, "密码哈希失败")
		return
	}
	if err := models.UpdateUserPassword(dbUser.ID, newHash); err != nil {
		api.Fail(c, api.CodeServerError, "更新密码失败")
		return
	}

	// 审计埋点：密码修改
	audit.RecordAudit(c, "change_password", "/admin/users/change-password", map[string]interface{}{
		"username": username,
	}, "success")

	api.Success(c, "密码修改成功", nil)
}

// DisableUser 禁用用户（软删除：设置 status=0）。
//
// DELETE /admin/users/:id
func DisableUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "无效的用户 ID")
		return
	}

	// 查询用户是否存在
	existing, err := models.GetUserByID(id)
	if err != nil {
		api.Fail(c, api.CodeNotFound, "用户不存在")
		return
	}

	if err := models.DeleteUser(existing.ID); err != nil {
		api.Fail(c, api.CodeServerError, "禁用用户失败")
		return
	}
	api.Success(c, "禁用用户成功", nil)
}
