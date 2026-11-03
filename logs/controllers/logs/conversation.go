// Package logs 子文件：AI 诊断会话 API（FR-901）。
//
// 提供会话 CRUD：创建会话、追加消息、查询历史、删除会话。
// 对话式工作台通过此组接口实现多轮上下文持久化。
package logs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// ConvCreateReq 创建会话请求。
type ConvCreateReq struct {
	Username string `json:"username"`
	Title    string `json:"title"`
}

// ConvCreateResp 创建会话响应。
type ConvCreateResp struct {
	ConvID string `json:"convId"`
}

// ConvAppendReq 追加消息请求。
type ConvAppendReq struct {
	ConvID       string `json:"convId"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	ToolCalls    string `json:"toolCalls"`
	EvidenceRefs string `json:"evidenceRefs"`
	TokenCount   int    `json:"tokenCount"`
}

// generateConvID 生成唯一会话 ID。
func generateConvID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("conv_%d", time.Now().UnixNano())
	}
	return "conv_" + hex.EncodeToString(b)
}

// ConvCreate 创建新会话。
//
// @Summary 创建会话
// @Description 创建一个新的 AI 诊断会话，返回会话 ID。
// @Tags Conversation
// @Accept json
// @Produce json
// @Param body body ConvCreateReq true "username 可选，title 可选"
// @Success 200 {object} api.Response{data=ConvCreateResp} "convId = 会话 ID"
// @Failure 400 {object} api.Response "参数错误"
// @Failure 500 {object} api.Response "创建失败"
// @Router /logs/conversation/create [post]
func ConvCreate(c *gin.Context) {
	var req ConvCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 从 session 获取 username（优先）或请求体
	username := req.Username
	if username == "" {
		username = c.GetString("username")
	}

	conv := &models.TConversation{
		ConvId:   generateConvID(),
		Username: username,
		Title:    req.Title,
		Status:   "active",
	}
	if conv.Title == "" {
		conv.Title = "新会话 " + time.Now().Format("01-02 15:04")
	}

	// 查询 user_id（如果用户存在于 t_user 表）
	if user, err := models.GetUserByUsername(username); err == nil && user.ID != 0 {
		conv.UserId = user.ID
	}

	if err := models.CreateConversation(conv); err != nil {
		api.Fail(c, api.CodeServerError, "创建会话失败: "+err.Error())
		return
	}

	api.Success(c, "会话创建成功", ConvCreateResp{ConvID: conv.ConvId})
}

// ConvList 查询当前用户的会话列表（分页）。
//
// @Summary 会话列表
// @Description 分页查询当前用户的 AI 诊断会话列表。
// @Tags Conversation
// @Produce json
// @Param pageNo query int false "页码（默认 1）"
// @Param pageSize query int false "每页条数（默认 20）"
// @Success 200 {object} api.Paginated{data=[]models.TConversation}
// @Failure 401 {object} api.Response "未登录"
// @Failure 404 {object} api.Response "用户不存在"
// @Failure 500 {object} api.Response "查询失败"
// @Router /logs/conversation/list [get]
func ConvList(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		api.Fail(c, api.CodeUnauthorized, "未登录")
		return
	}

	user, err := models.GetUserByUsername(username)
	if err != nil || user.ID == 0 {
		api.Fail(c, api.CodeNotFound, "用户不存在")
		return
	}

	pageNo := parseIntDefault(c.Query("pageNo"), 1)
	pageSize := parseIntDefault(c.Query("pageSize"), 20)

	page, err := models.ListConversations(user.ID, pageNo, pageSize)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询会话列表失败: "+err.Error())
		return
	}

	api.Success(c, "查询成功", page)
}

// ConvMessages 查询会话的全部消息。
//
// @Summary 会话消息
// @Description 查询指定会话的全部消息记录。
// @Tags Conversation
// @Produce json
// @Param convId path string true "会话 ID"
// @Success 200 {object} api.Response{data=[]models.TConversationMsg}
// @Failure 400 {object} api.Response "convId 不能为空"
// @Failure 500 {object} api.Response "查询失败"
// @Router /logs/conversation/{convId}/messages [get]
func ConvMessages(c *gin.Context) {
	convID := c.Param("convId")
	if convID == "" {
		api.Fail(c, api.CodeBadRequest, "convId 不能为空")
		return
	}

	msgs, err := models.GetMessagesByConvID(convID)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询消息失败: "+err.Error())
		return
	}

	api.Success(c, "查询成功", msgs)
}

// ConvAppend 追加一条消息到会话。
//
// @Summary 追加消息
// @Description 向指定会话追加一条消息，用于多轮对话上下文持久化。
// @Tags Conversation
// @Accept json
// @Produce json
// @Param body body ConvAppendReq true "convId/role/content 均为必填"
// @Success 200 {object} api.Response{data=map[string]interface{}} "turn = 轮次, id = 消息 ID"
// @Failure 400 {object} api.Response "参数错误"
// @Failure 404 {object} api.Response "会话不存在"
// @Failure 500 {object} api.Response "追加失败"
// @Router /logs/conversation/append [post]
func ConvAppend(c *gin.Context) {
	var req ConvAppendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.ConvID == "" || req.Role == "" || req.Content == "" {
		api.Fail(c, api.CodeBadRequest, "convId/role/content 不能为空")
		return
	}

	// 验证会话存在
	conv, err := models.GetConversationByConvID(req.ConvID)
	if err != nil || conv.Id == 0 {
		api.Fail(c, api.CodeNotFound, "会话不存在")
		return
	}

	// 获取下一个 turn
	nextTurn, err := models.GetNextTurn(req.ConvID)
	if err != nil {
		api.Fail(c, api.CodeServerError, "获取 turn 失败: "+err.Error())
		return
	}

	msg := &models.TConversationMsg{
		ConvId:       req.ConvID,
		Turn:         nextTurn,
		Role:         req.Role,
		Content:      req.Content,
		ToolCalls:    req.ToolCalls,
		EvidenceRefs: req.EvidenceRefs,
		TokenCount:   req.TokenCount,
	}
	if err := models.AppendMessage(msg); err != nil {
		api.Fail(c, api.CodeServerError, "追加消息失败: "+err.Error())
		return
	}

	// 更新会话统计
	newCount := conv.RoleCount + 1
	newTokens := conv.TokenUsed + req.TokenCount
	_ = models.UpdateConversationStats(req.ConvID, newTokens, newCount)

	api.Success(c, "消息追加成功", gin.H{"turn": nextTurn, "id": msg.Id})
}

// ConvDelete 删除会话及其全部消息。
//
// @Summary 删除会话
// @Description 删除指定会话及其全部消息记录。
// @Tags Conversation
// @Produce json
// @Param convId path string true "会话 ID"
// @Success 200 {object} api.Response "删除成功"
// @Failure 400 {object} api.Response "convId 不能为空"
// @Failure 500 {object} api.Response "删除失败"
// @Router /logs/conversation/{convId} [delete]
func ConvDelete(c *gin.Context) {
	convID := c.Param("convId")
	if convID == "" {
		api.Fail(c, api.CodeBadRequest, "convId 不能为空")
		return
	}

	if err := models.DeleteConversation(convID); err != nil {
		api.Fail(c, api.CodeServerError, "删除会话失败: "+err.Error())
		return
	}

	api.Success(c, "会话删除成功", nil)
}

// ConvUpdateTitle 更新会话标题。
//
// @Summary 更新会话标题
// @Description 更新指定会话的标题。
// @Tags Conversation
// @Accept json
// @Produce json
// @Param convId path string true "会话 ID"
// @Param body body object{title=string} true "title: 新标题（必填）"
// @Success 200 {object} api.Response "更新成功"
// @Failure 400 {object} api.Response "参数错误"
// @Failure 500 {object} api.Response "更新失败"
// @Router /logs/conversation/{convId}/title [put]
func ConvUpdateTitle(c *gin.Context) {
	convID := c.Param("convId")
	if convID == "" {
		api.Fail(c, api.CodeBadRequest, "convId 不能为空")
		return
	}

	var req struct {
		Title string `json:"title" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}

	if err := models.UpdateConversationTitle(convID, req.Title); err != nil {
		api.Fail(c, api.CodeServerError, "更新标题失败: "+err.Error())
		return
	}

	api.Success(c, "标题更新成功", nil)
}

// parseIntDefault 解析整数参数，失败时返回默认值。
func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}

// 确保 net/http 被使用（避免 import 问题）。
var _ = http.StatusOK
