// Package logs 子文件：日志模板查询（自动补全）。
//
// 提供模板关键词搜索接口，前端输入框实时联想补全。
// 数据来源为 t_log_template 表（由 Drain 解析引擎写入）。
package logs

import (
	"strconv"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// SearchTemplates GET /logs/templates/search — 模板自动补全。
//
// Query: q(str, required) keyword 关键字; limit(int, default=10, max=20)
//
// @Summary 模板自动补全
// @Description 按关键字搜索日志模板（template_text / sample_line 模糊匹配），返回高频模板列表。
// @Tags LogSearch
// @Produce json
// @Param q query string true "搜索关键字（最少 1 字符）"
// @Param limit query int false "返回条数（默认 10，最大 20）"
// @Success 200 {object} api.Response{data=map[string]any} "list = []TLogTemplate"
// @Router /logs/templates/search [get]
func SearchTemplates(c *gin.Context) {
	keyword := c.Query("q")
	if len(keyword) < 1 {
		api.Fail(c, api.CodeBadRequest, "关键字最少 1 字符")
		return
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil || limit <= 0 || limit > 20 {
		limit = 10
	}

	templates, err := models.SearchTemplates(keyword, limit)
	if err != nil {
		api.Fail(c, api.CodeServerError, "模板搜索失败: "+err.Error())
		return
	}

	api.Success(c, "搜索成功", gin.H{
		"list":  templates,
		"total": len(templates),
	})
}
