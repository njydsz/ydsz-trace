// Package admin 子文件：模型能力配置管理控制器。
//
// 安全说明：
//   - 写操作（新增/更新/删除/启停）仅 admin 可执行
//   - 列表查询已登录用户均可访问
//   - api_key 空字符串表示不更新密钥
package admin

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// ========== 请求结构体 ==========

// modelCapabilityReq 新增/更新模型能力配置请求体。
type modelCapabilityReq struct {
	Id          int64   `json:"id"`
	Name        string  `json:"name" binding:"required"`
	Provider    string  `json:"provider" binding:"required"`
	ModelId     string  `json:"modelId" binding:"required"`
	ApiKey      string  `json:"apiKey"`
	BaseUrl     string  `json:"baseUrl"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"maxTokens"`
	Enabled     int     `json:"enabled"`
	Capability  string  `json:"capability"`
}

// modelCapabilityQueryReq 列表查询请求（query string 形式）。
type modelCapabilityQueryReq struct {
	PageNo     int    `form:"pageNo"`
	PageSize   int    `form:"pageSize"`
	Keyword    string `form:"keyword"`
	Capability string `form:"capability"`
}

// modelCapabilityIDReq ID 请求体（启停/删除）。
type modelCapabilityIDReq struct {
	ID int64 `json:"id" binding:"required"`
}

// modelCapabilityToggleReq 启用/禁用请求体。
type modelCapabilityToggleReq struct {
	ID      int64 `json:"id" binding:"required"`
	Enabled int   `json:"enabled"`
}

// ========== Handlers ==========

// @Summary 查询模型能力配置列表
// @Description 分页查询模型能力配置列表，支持按能力类型和关键词过滤。
// @Tags ModelCapability
// @Accept json
// @Produce json
// @Param pageNo query int false "页码（默认 1）"
// @Param pageSize query int false "每页条数（默认 20）"
// @Param keyword query string false "名称/模型/提供商 关键词搜索"
// @Param capability query string false "能力类型过滤（chat / embedding / rerank）"
// @Success 200 {object} api.Response "配置分页列表"
// @Failure 401 {object} api.Response "未登录"
// @Router /admin/model-capability/list [get]
func ListModelCapabilities(c *gin.Context) {
	var q modelCapabilityQueryReq
	_ = c.ShouldBindQuery(&q)
	if q.PageNo <= 0 {
		q.PageNo = 1
	}
	if q.PageSize <= 0 || q.PageSize > 100 {
		q.PageSize = 20
	}

	page, err := models.ListModelCapabilities(q.Keyword, q.Capability, q.PageNo, q.PageSize)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询模型能力配置列表失败")
		return
	}
	api.Paginated(c, "查询成功", page.List, page.TotalCount, q.PageNo, q.PageSize)
}

// @Summary 获取模型能力配置详情
// @Description 根据 ID 查询单个模型能力配置。
// @Tags ModelCapability
// @Accept json
// @Produce json
// @Param id path int true "配置 ID"
// @Success 200 {object} api.Response "配置详情"
// @Failure 401 {object} api.Response "未登录"
// @Failure 404 {object} api.Response "配置不存在"
// @Router /admin/model-capability/{id} [get]
func GetModelCapability(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "无效的配置 ID")
		return
	}

	m, err := models.GetModelCapabilityByID(id)
	if err != nil {
		api.Fail(c, api.CodeNotFound, "模型能力配置不存在")
		return
	}
	api.Success(c, "查询成功", m)
}

// @Summary 新增模型能力配置
// @Description 新增一个 AI 模型能力配置（chat/embedding/rerank）。
// @Tags ModelCapability
// @Accept json
// @Produce json
// @Param body body modelCapabilityReq true "配置信息"
// @Success 200 {object} api.Response "创建成功"
// @Failure 400 {object} api.Response "参数错误或名称已存在"
// @Failure 401 {object} api.Response "未登录"
// @Router /admin/model-capability [post]
func AddModelCapability(c *gin.Context) {
	var req modelCapabilityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: 名称、提供商、模型 ID 为必填项")
		return
	}

	// 校验能力类型
	if req.Capability == "" {
		req.Capability = "chat"
	}
	if !isValidCapability(req.Capability) {
		api.Fail(c, api.CodeBadRequest, "无效的能力类型，允许值: chat / embedding / rerank")
		return
	}

	// 名称唯一性校验
	if models.ModelCapabilityExists(req.Name, 0) {
		api.Fail(c, api.CodeBadRequest, "配置名称已存在")
		return
	}

	m := &models.LogModelCapability{
		Name:        req.Name,
		Provider:    req.Provider,
		ModelId:     req.ModelId,
		ApiKey:      req.ApiKey,
		BaseUrl:     req.BaseUrl,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Enabled:     req.Enabled,
		Capability:  req.Capability,
		CreatedBy:   session.GetString(c, "username"),
	}
	if m.Enabled == 0 {
		m.Enabled = 1
	}
	if m.Temperature <= 0 {
		m.Temperature = 0.7
	}
	if m.MaxTokens <= 0 {
		m.MaxTokens = 4096
	}

	id, err := models.AddModelCapability(m)
	if err != nil {
		api.Fail(c, api.CodeServerError, "创建模型能力配置失败")
		return
	}
	api.Success(c, "创建成功", gin.H{"id": id})
}

// @Summary 更新模型能力配置
// @Description 更新模型能力配置。apiKey 为空时不更新密钥字段。
// @Tags ModelCapability
// @Accept json
// @Produce json
// @Param body body modelCapabilityReq true "配置信息（id 必填）"
// @Success 200 {object} api.Response "更新成功"
// @Failure 400 {object} api.Response "参数错误或名称已存在"
// @Failure 401 {object} api.Response "未登录"
// @Failure 404 {object} api.Response "配置不存在"
// @Router /admin/model-capability [put]
func UpdateModelCapability(c *gin.Context) {
	var req modelCapabilityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: ID 必填，名称/提供商/模型 ID 不可为空")
		return
	}
	if req.Id <= 0 {
		api.Fail(c, api.CodeBadRequest, "无效的配置 ID")
		return
	}

	// 查询配置是否存在
	existing, err := models.GetModelCapabilityByID(req.Id)
	if err != nil {
		api.Fail(c, api.CodeNotFound, "模型能力配置不存在")
		return
	}

	// 校验能力类型
	if req.Capability != "" && !isValidCapability(req.Capability) {
		api.Fail(c, api.CodeBadRequest, "无效的能力类型，允许值: chat / embedding / rerank")
		return
	}

	// 名称唯一性校验（排除自身）
	if req.Name != "" && models.ModelCapabilityExists(req.Name, req.Id) {
		api.Fail(c, api.CodeBadRequest, "配置名称已存在")
		return
	}

	// 更新字段
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Provider != "" {
		existing.Provider = req.Provider
	}
	if req.ModelId != "" {
		existing.ModelId = req.ModelId
	}
	// apiKey 为空字符串表示不更新
	if req.ApiKey != "" {
		existing.ApiKey = req.ApiKey
	}
	existing.BaseUrl = req.BaseUrl
	existing.Enabled = req.Enabled
	if req.Capability != "" {
		existing.Capability = req.Capability
	}
	if req.Temperature > 0 {
		existing.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		existing.MaxTokens = req.MaxTokens
	}

	if err := models.UpdateModelCapability(existing); err != nil {
		api.Fail(c, api.CodeServerError, "更新模型能力配置失败")
		return
	}
	api.Success(c, "更新成功", nil)
}

// @Summary 删除模型能力配置
// @Description 硬删除模型能力配置。
// @Tags ModelCapability
// @Accept json
// @Produce json
// @Param body body modelCapabilityIDReq true "配置 ID"
// @Success 200 {object} api.Response "删除成功"
// @Failure 400 {object} api.CodeBadRequest "参数错误"
// @Failure 401 {object} api.Response "未登录"
// @Router /admin/model-capability/delete [post]
func DeleteModelCapability(c *gin.Context) {
	var req modelCapabilityIDReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: ID 必填")
		return
	}
	if req.ID <= 0 {
		api.Fail(c, api.CodeBadRequest, "无效的配置 ID")
		return
	}

	if err := models.DeleteModelCapability(req.ID); err != nil {
		api.Fail(c, api.CodeServerError, "删除模型能力配置失败")
		return
	}
	api.Success(c, "删除成功", nil)
}

// @Summary 启用/禁用模型能力配置
// @Description 切换模型能力配置的启用状态。
// @Tags ModelCapability
// @Accept json
// @Produce json
// @Param body body modelCapabilityToggleReq true "id + enabled"
// @Success 200 {object} api.Response "切换成功"
// @Failure 400 {object} api.CodeBadRequest "参数错误"
// @Failure 401 {object} api.Response "未登录"
// @Failure 404 {object} api.Response "配置不存在"
// @Router /admin/model-capability/toggle [post]
func ToggleModelCapability(c *gin.Context) {
	var req modelCapabilityToggleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: ID 必填")
		return
	}
	if req.ID <= 0 {
		api.Fail(c, api.CodeBadRequest, "无效的配置 ID")
		return
	}

	// 查询配置是否存在
	existing, err := models.GetModelCapabilityByID(req.ID)
	if err != nil {
		api.Fail(c, api.CodeNotFound, "模型能力配置不存在")
		return
	}
	existing.Enabled = req.Enabled
	if err := models.UpdateModelCapability(existing); err != nil {
		api.Fail(c, api.CodeServerError, "切换模型能力配置状态失败")
		return
	}
	api.Success(c, "操作成功", nil)
}

// modelCapabilitySelfCheckReq 连通性自检请求体。
type modelCapabilitySelfCheckReq struct {
	ID int64 `json:"id" binding:"required"`
}

// selfCheckResult 自检结果。
type selfCheckResult struct {
	Reachable bool   `json:"reachable"`
	LatencyMs int64  `json:"latencyMs"`
	Message   string `json:"message"`
}

// @Summary 连通性自检
// @Description 对指定模型能力配置进行一次轻量级 API 探测，返回是否可达、往返延迟与提示信息。
// @Tags ModelCapability
// @Accept json
// @Produce json
// @Param body body modelCapabilitySelfCheckReq true "配置 ID"
// @Success 200 {object} api.Response "自检结果"
// @Failure 400 {object} api.Response "参数错误"
// @Failure 401 {object} api.Response "未登录"
// @Failure 404 {object} api.Response "配置不存在"
// @Router /admin/model-capability/selfcheck [post]
func SelfCheckModelCapability(c *gin.Context) {
	var req modelCapabilitySelfCheckReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: ID 必填")
		return
	}
	if req.ID <= 0 {
		api.Fail(c, api.CodeBadRequest, "无效的配置 ID")
		return
	}

	cfg, err := models.GetModelCapabilityByID(req.ID)
	if err != nil {
		api.Fail(c, api.CodeNotFound, "模型能力配置不存在")
		return
	}

	result := performSelfCheck(cfg)
	api.Success(c, "自检完成", result)
}

// performSelfCheck 根据能力类型选择合适的探测方式。
func performSelfCheck(cfg *models.LogModelCapability) selfCheckResult {
	switch cfg.Capability {
	case "embedding":
		return selfCheckEmbedding(cfg)
	case "rerank":
		return selfCheckRerank(cfg)
	default:
		return selfCheckChat(cfg)
	}
}

// selfCheckChat 对话模型：请求 /v1/models 或 chat/completions 接口。
func selfCheckChat(cfg *models.LogModelCapability) selfCheckResult {
	url := resolveURL(cfg, "/v1/models")
	if url == "" {
		return selfCheckResult{Reachable: false, Message: "无法构造 API 端点，请检查 baseUrl"}
	}
	return httpPing(url, cfg)
}

// selfCheckEmbedding 嵌入模型：调用 /v1/embeddings 接口发送一个 token 检测连通性。
func selfCheckEmbedding(cfg *models.LogModelCapability) selfCheckResult {
	url := resolveURL(cfg, "/v1/embeddings")
	if url == "" {
		return selfCheckResult{Reachable: false, Message: "无法构造 API 端点，请检查 baseUrl"}
	}
	return httpPing(url, cfg)
}

// selfCheckRerank 重排序模型：尝试 POST 一个轻量请求到 /rerank 路径。
func selfCheckRerank(cfg *models.LogModelCapability) selfCheckResult {
	url := resolveURL(cfg, "/rerank")
	if url == "" {
		return selfCheckResult{Reachable: false, Message: "无法构造 API 端点，请检查 baseUrl"}
	}
	return httpPing(url, cfg)
}

// resolveURL 拼接完整 API 端点 URL。
func resolveURL(cfg *models.LogModelCapability, path string) string {
	base := cfg.BaseUrl
	if base == "" {
		// 默认 OpenAI 兼容端点
		switch cfg.Provider {
		case "openai":
			base = "https://api.openai.com"
		case "anthropic":
			base = "https://api.anthropic.com"
		case "ollama":
			base = "http://localhost:11434"
		default:
			return ""
		}
	}
	// 去除末尾斜杠后拼接
	if len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + path
}

// httpPing 发送轻量 GET 请求测试可达性，返回延迟和状态。
func httpPing(url string, cfg *models.LogModelCapability) selfCheckResult {
	return probeEndpoint(url, cfg, false)
}

// probeEndpointProbeTimeout 探测请求超时。
const probeEndpointProbeTimeout = 10 * time.Second

// probeEndpoint 执行实际 HTTP 探测。
func probeEndpoint(url string, cfg *models.LogModelCapability, fullTest bool) selfCheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), probeEndpointProbeTimeout)
	defer cancel()

	method := http.MethodGet
	var body io.Reader
	if fullTest && (cfg.Capability == "embedding" || cfg.Capability == "rerank") {
		method = http.MethodPost
		body = nil
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return selfCheckResult{Reachable: false, Message: "构造探测请求失败: " + err.Error()}
	}
	if cfg.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.ApiKey)
	}
	req.Header.Set("User-Agent", "YdszTrace/2.0 self-check")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return selfCheckResult{Reachable: false, LatencyMs: latency, Message: "连接失败: " + err.Error()}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		return selfCheckResult{
			Reachable: true,
			LatencyMs: latency,
			Message:   "OK (HTTP " + strconv.Itoa(resp.StatusCode) + ")",
		}
	}
	return selfCheckResult{
		Reachable: false,
		LatencyMs: latency,
		Message:   "异常响应 (HTTP " + strconv.Itoa(resp.StatusCode) + ")",
	}
}

// isValidCapability 校验能力类型是否合法。
func isValidCapability(capability string) bool {
	switch capability {
	case "chat", "embedding", "rerank":
		return true
	}
	return false
}
