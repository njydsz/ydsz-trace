// Package logs 子文件：案例库 API（FR-907）。
//
// 提供案例保存、症状搜索、分页列表三个端点：
//   - POST /logs/cases/save     从对话工作台「存入案例库」触发
//   - GET  /logs/cases/search   按症状关键词模糊搜索
//   - GET  /logs/cases/list     分页列出案例
package logs

import (
	"net/http"
	"strconv"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// CaseSaveReq 保存案例请求。
type CaseSaveReq struct {
	Title     string   `json:"title" binding:"required"`
	Symptoms  []string `json:"symptoms"`
	RootCause string   `json:"rootCause"`
	Solution  string   `json:"solution"`
	TagList   string   `json:"tagList"`
	Source    string   `json:"source"` // manual / conversation / feedback
	ConvID    string   `json:"convId"`
	CreatedBy string   `json:"createdBy"`
}

// CaseSaveHandler 保存案例到案例库。
//
// @Summary 保存案例
// @Description 将根因分析结果或手动输入的案例保存到案例库，供后续相似问题检索。
// @Tags CaseLibrary
// @Accept json
// @Produce json
// @Param body body CaseSaveReq true "案例内容：title 与 symptoms 为必填"
// @Success 200 {object} api.Response{data=map[string]int64} "id = 案例 ID"
// @Failure 400 {object} api.Response "参数错误"
// @Failure 500 {object} api.Response "保存失败"
// @Router /logs/cases/save [post]
func CaseSaveHandler(c *gin.Context) {
	var req CaseSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: title 为必填项")
		return
	}
	if len(req.Symptoms) == 0 {
		api.Fail(c, api.CodeBadRequest, "symptoms 不能为空")
		return
	}

	source := models.CaseSource(req.Source)
	if source == "" {
		source = models.CaseSourceManual
	}
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = c.GetString("username")
	}

	entry := models.CaseEntry{
		Title:     req.Title,
		Symptoms:  req.Symptoms,
		RootCause: req.RootCause,
		Solution:  req.Solution,
		TagList:   req.TagList,
		Source:    source,
		ConvID:    req.ConvID,
		CreatedBy: createdBy,
	}

	id, err := models.SaveCase(entry)
	if err != nil {
		api.Fail(c, api.CodeServerError, "保存案例失败: "+err.Error())
		return
	}

	api.Success(c, "案例保存成功", gin.H{"id": id})
}

// CaseSearchHandler 按症状关键词搜索案例。
//
// @Summary 搜索案例
// @Description 按症状关键词模糊搜索案例库，返回匹配案例列表。
// @Tags CaseLibrary
// @Produce json
// @Param keyword query string true "搜索关键词"
// @Param limit query int false "返回条数（默认 10，最大 50）"
// @Success 200 {object} api.Response{data=[]models.CaseEntry}
// @Failure 400 {object} api.Response "keyword 不能为空"
// @Failure 500 {object} api.Response "搜索失败"
// @Router /logs/cases/search [get]
func CaseSearchHandler(c *gin.Context) {
	keyword := c.DefaultQuery("keyword", "")
	if keyword == "" {
		api.Fail(c, api.CodeBadRequest, "keyword 不能为空")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	cases, err := models.SearchCasesBySymptoms(keyword, limit)
	if err != nil {
		api.Fail(c, api.CodeServerError, "搜索案例失败: "+err.Error())
		return
	}

	api.Success(c, "搜索成功", cases)
}

// CaseListHandler 分页列出案例。
//
// @Summary 案例列表
// @Description 分页列出案例库中的全部案例。
// @Tags CaseLibrary
// @Produce json
// @Param pageNo query int false "页码（默认 1）"
// @Param pageSize query int false "每页条数（默认 20，最大 200）"
// @Success 200 {object} api.Paginated{data=[]models.CaseEntry}
// @Failure 500 {object} api.Response "查询失败"
// @Router /logs/cases/list [get]
func CaseListHandler(c *gin.Context) {
	pageNo, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	offset := (pageNo - 1) * pageSize

	cases, total, err := models.ListCases(pageSize, offset)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询案例列表失败: "+err.Error())
		return
	}

	api.Paginated(c, "查询成功", cases, pageNo, pageSize, total)
}

// 确保 net/http 被使用。
var _ = http.StatusOK
