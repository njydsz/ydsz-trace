package alert

import (
	"context"
	"errors"
	"strconv"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// ============ 请求/响应结构 ============

type alertRuleReq struct {
	ID          int64  `json:"id"`
	Name        string `json:"name" binding:"required"`
	ItemID      int64  `json:"itemId" binding:"required"`
	ClientID    int64  `json:"clientId"`
	KeyWord     string `json:"keyWord"`
	Regex       bool   `json:"regex"`
	Level       string `json:"level"`
	Threshold   int    `json:"threshold"`
	IntervalSec int    `json:"intervalSec"`
	WebhookURL  string `json:"webhookUrl" binding:"required,url"`
	Enabled     int    `json:"enabled"`
}

type idReq struct {
	ID int64 `json:"id" binding:"required"`
}

type toggleReq struct {
	ID      int64 `json:"id" binding:"required"`
	Enabled int   `json:"enabled"`
}

// queryReq 列表查询请求（query string 形式）。
type queryReq struct {
	PageNo   int    `form:"pageNo"`
	PageSize int    `form:"pageSize"`
	Keyword  string `form:"keyword"`
	ruleID   int64
	status   string
}

// ===========: Handlers ============

// ListRules GET /logs/alerts/rules?pageNo=&pageSize=&keyword=
func ListRules(c *gin.Context) {
	var q queryReq
	_ = c.ShouldBindQuery(&q)
	// 防止大量用户初始化时拖垮 db
	if q.PageNo <= 0 {
		q.PageNo = 1
	}
	if q.PageSize <= 0 || q.PageSize > 100 {
		q.PageSize = 20
	}
	p := models.QueryAlertRules(q.PageNo, q.PageSize, q.Keyword)
	list, ok := p.List.([]models.AlertRule)
	if ok {
		for i := range list {
			models.FillRuleItemName(&list[i])
		}
	}
	api.Success(c, "查询成功", p)
}

// GetRule GET /logs/alerts/rules/:id
func GetRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "id 非法")
		return
	}
	rule, err := models.GetAlertRule(id)
	if err != nil || rule.ID == 0 {
		api.Fail(c, api.CodeNotFound, "规则不存在")
		return
	}
	models.FillRuleItemName(&rule)
	api.Success(c, "查询成功", rule)
}

// AddRule POST /logs/alerts/rules
func AddRule(c *gin.Context) {
	var req alertRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	userName := session.GetString(c, "username")
	rule := models.AlertRule{
		Name:        req.Name,
		ItemID:      req.ItemID,
		ClientID:    req.ClientID,
		KeyWord:     req.KeyWord,
		Regex:       req.Regex,
		Level:       req.Level,
		Threshold:   req.Threshold,
		IntervalSec: req.IntervalSec,
		WebhookURL:  req.WebhookURL,
		Enabled:     req.Enabled,
		CreatedBy:   userName,
	}
	if err := clampRule(&rule); err != nil {
		api.Fail(c, api.CodeBadRequest, err.Error())
		return
	}
	id, err := models.InsertAlertRule(&rule)
	if err != nil {
		api.Fail(c, api.CodeServerError, "新增失败")
		return
	}
	api.Success(c, "新增成功", gin.H{"id": id})
}

// UpdateRule PUT /logs/alerts/rules/:id
func UpdateRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "id 非法")
		return
	}
	var req alertRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: "+err.Error())
		return
	}
	existing, err := models.GetAlertRule(id)
	if err != nil || existing.ID == 0 {
		api.Fail(c, api.CodeNotFound, "规则不存在")
		return
	}
	existing.Name = req.Name
	existing.ClientID = req.ClientID
	existing.KeyWord = req.KeyWord
	existing.Regex = req.Regex
	existing.Level = req.Level
	existing.Threshold = req.Threshold
	existing.IntervalSec = req.IntervalSec
	existing.WebhookURL = req.WebhookURL
	existing.Enabled = req.Enabled
	if err := clampRule(&existing); err != nil {
		api.Fail(c, api.CodeBadRequest, err.Error())
		return
	}
	if err := models.UpdateAlertRule(&existing); err != nil {
		api.Fail(c, api.CodeServerError, "更新失败")
		return
	}
	api.Success(c, "更新成功", gin.H{"id": id})
}

// DeleteRule DELETE /logs/alerts/rules/:id
func DeleteRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "id 非法")
		return
	}
	n, err := models.DeleteAlertRule(id)
	if err != nil || n == 0 {
		api.Fail(c, api.CodeNotFound, "规则不存在或已删除")
		return
	}
	api.Success(c, "删除成功", gin.H{"id": id})
}

// ToggleRule POST /logs/alerts/rules/toggle  body: { id, enabled }
func ToggleRule(c *gin.Context) {
	var req toggleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	existing, err := models.GetAlertRule(req.ID)
	if err != nil || existing.ID == 0 {
		api.Fail(c, api.CodeNotFound, "规则不存在")
		return
	}
	existing.Enabled = req.Enabled
	if err := models.UpdateAlertRule(&existing); err != nil {
		api.Fail(c, api.CodeServerError, "状态切换失败")
		return
	}
	api.Success(c, "状态已切换", gin.H{"id": req.ID, "enabled": req.Enabled})
}

// TestFire POST /logs/alerts/rules/test  body: { id }  手动触发一次评估。
func TestFire(c *gin.Context) {
	var req idReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	rule, err := models.GetAlertRule(req.ID)
	if err != nil || rule.ID == 0 {
		api.Fail(c, api.CodeNotFound, "规则不存在")
		return
	}
	models.FillRuleItemName(&rule)
	go evaluateAndFire(context.Background(), rule, time.Now())
	api.Success(c, "已触发测试执行", gin.H{"ruleId": rule.ID})
}

// ListEvents GET /logs/alerts/events?pageNo=&pageSize=&ruleId=&status=&keyword=
func ListEvents(c *gin.Context) {
	ruleID, _ := strconv.ParseInt(c.Query("ruleId"), 10, 64)
	status := c.Query("status")
	pageNo, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	p := models.QueryAlertEvents(pageNo, pageSize, ruleID, status)
	api.Success(c, "查询成功", p)
}

// DeleteEvent DELETE /logs/alerts/events/:id
func DeleteEvent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "id 非法")
		return
	}
	n, err := models.DeleteAlertEvent(id)
	if err != nil || n == 0 {
		api.Fail(c, api.CodeNotFound, "记录不存在或已删除")
		return
	}
	api.Success(c, "删除成功", gin.H{"id": id})
}

// Quota GET /logs/alerts/quota  返回规则数 / 近 24h 成功 / 失败数。
func Quota(c *gin.Context) {
	enabled, firedToday, failedToday := models.AlertRulesQuota()
	api.Success(c, "查询成功", gin.H{
		"enabled":     enabled,
		"firedToday":  firedToday,
		"failedToday": failedToday,
	})
}

// clampRule 校验并限制阈值/间隔在合理范围。
func clampRule(r *models.AlertRule) error {
	if r.Name == "" {
		return errors.New("name 不能为空")
	}
	if r.Threshold <= 0 {
		r.Threshold = 1
	}
	if r.Threshold > 10000 {
		r.Threshold = 10000
	}
	if r.IntervalSec < 60 {
		r.IntervalSec = 60
	}
	if r.IntervalSec > 86400 {
		r.IntervalSec = 86400
	}
	if r.WebhookURL == "" {
		return errors.New("webhook_url 不能为空")
	}
	return nil
}
