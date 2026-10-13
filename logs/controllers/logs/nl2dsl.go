// Package logs 包含日志检索相关控制器。
//
// 本文件实现 NL2DSL（Natural Language to Domain-Specific Language）服务。
// 将用户的自然语言检索意图解析为结构化的日志检索 DSL 参数。
//
// 解析能力：
//   - 日志级别提取（ERROR/WARN/INFO/DEBUG/FATAL）
//   - 时间范围解析（今天/N小时/N天/具体时间区间）
//   - 关键词提取（引号内文本/剩余文本）
//   - 意图分类（error_search/time_range/keyword_search/general）
//
// 路由：POST /logs/nl2dsl/parse
// 鉴权：RequireOperator
package logs

import (
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"ydsz-trace/pkg/api"
)

// Nl2DslParseReq NL2DSL 解析请求。
type Nl2DslParseReq struct {
	// Query 用户自然语言输入
	Query string `json:"query" binding:"required"`
}

// Nl2DslParseResp NL2DSL 解析响应。
type Nl2DslParseResp struct {
	// UserIntent 用户原始意图
	UserIntent string `json:"userIntent"`
	// Level 日志级别
	Level string `json:"level"`
	// Date 日期 YYYYMMDD
	Date string `json:"date"`
	// StartTime 起始时间 HH:MM:SS
	StartTime string `json:"startTime"`
	// EndTime 结束时间 HH:MM:SS
	EndTime string `json:"endTime"`
	// Key 关键词
	Key string `json:"key"`
	// TimeRangeDetail 时间范围描述
	TimeRangeDetail string `json:"timeRangeDetail"`
	// DslConditions DSL 条件表达式
	DslConditions string `json:"dslConditions"`
	// IntentType 意图类型
	IntentType string `json:"intentType"`
}

// 常量定义。
const (
	// LEVEL_ERROR / LEVEL_WARN / LEVEL_INFO / LEVEL_DEBUG / FATAL
	LEVEL_ERROR = "ERROR"
	LEVEL_WARN  = "WARN"
	LEVEL_INFO  = "INFO"
	LEVEL_DEBUG = "DEBUG"
	LEVEL_FATAL = "FATAL"

	// ITEM 意图类型
	INTENT_ERROR_SEARCH  = "error_search"
	INTENT_TIME_RANGE    = "time_range"
	INTENT_KEYWORD       = "keyword_search"
	INTENT_GENERAL       = "general"
)

// 正则预编译。
var (
	nlLevelPattern      = regexp.MustCompile(`(?i)\b(ERROR|WARN|INFO|DEBUG|FATAL)\b`)
	nlHourPattern       = regexp.MustCompile(`(?:近|最近|last)\s*(\d+)\s*小时`)
	nlDayPattern        = regexp.MustCompile(`(?:近|最近|last)\s*(\d+)\s*天`)
	nlQuotedPattern     = regexp.MustCompile(`["'']([^'']+)["'']`)
	nlCleanupTimeRef    = regexp.MustCompile(`(?:近\s*\d+\s*(?:小时|天)|最近\s*\d+\s*(?:小时|天)|今天|今日|日志|查[一查]?[下看]?[一下我]?[们]?|[查看]?[一一下]?[看我]?[们]?)`)
	nlCleanupLevel      = regexp.MustCompile(`(?i)\b(ERROR|WARN|INFO|DEBUG|FATAL)\b`)
)

// nl2dslParse 处理自然语言转 DSL 解析。
//
// @Summary NL2DSL 解析
// @Description 将自然语言检索意图解析为结构化日志检索 DSL 参数
// @Tags NL2DSL
// @Accept json
// @Produce json
// @Param request body Nl2DslParseReq true "自然语言查询"
// @Success 200 {object} api.Success{data=Nl2DslParseResp}
// @Router /logs/nl2dsl/parse [post]
// Nl2DslParse 是 nl2dslParse 的路由处理器包装。
func Nl2DslParse(c *gin.Context) {
	nl2dslParse(c)
}

func nl2dslParse(c *gin.Context) {
	var req Nl2DslParseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: "+err.Error())
		return
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		api.Fail(c, api.CodeBadRequest, "查询内容不能为空")
		return
	}

	resp := nl2dslBuildResponse(req.Query)
	api.Success(c, "NL2DSL 解析成功", resp)
}

// nl2dslBuildResponse 解析自然语言并构造响应。
func nl2dslBuildResponse(query string) Nl2DslParseResp {
	level := nl2dslExtractLevel(query)
	date, startTime, endTime, timeRangeDetail := nl2dslExtractTimeRange(query)
	key := nl2dslExtractKey(query)
	intentType := nl2dslDetectIntent(level, startTime, endTime, key)
	dslConditions := nl2dslFormatConditions(level, key, date, startTime, endTime)

	return Nl2DslParseResp{
		UserIntent:      query,
		Level:           level,
		Date:            date,
		StartTime:       startTime,
		EndTime:         endTime,
		Key:             key,
		TimeRangeDetail: timeRangeDetail,
		DslConditions:   dslConditions,
		IntentType:      intentType,
	}
}

// nl2dslExtractLevel 从自然语言中提取日志级别。
func nl2dslExtractLevel(text string) string {
	match := nlLevelPattern.FindString(text)
	if match == "" {
		return ""
	}
	return strings.ToUpper(match)
}

// nl2dslExtractTimeRange 解析时间范围。
func nl2dslExtractTimeRange(text string) (date, startTime, endTime, detail string) {
	now := time.Now()
	date = nlFormatDate(now)

	// 近 N 小时
	if m := nlHourPattern.FindStringSubmatch(text); m != nil {
		hours := nlParseInt(m[1])
		t := now.Add(-time.Duration(hours) * time.Hour)
		startTime = nlFormatTime(t)
		endTime = nlFormatTime(now)
		detail = nlFormatTime(t) + " - " + nlFormatTime(now)
		return
	}

	// 今天 / 今日
	if strings.Contains(text, "今天") || strings.Contains(text, "今日") {
		startTime = "00:00:00"
		endTime = nlFormatTime(now)
		detail = "今天 00:00:00 - " + nlFormatTime(now)
		return
	}

	// 近 N 天
	if m := nlDayPattern.FindStringSubmatch(text); m != nil {
		days := nlParseInt(m[1])
		t := now.Add(-time.Duration(days) * 24 * time.Hour)
		date = nlFormatDate(t)
		startTime = ""
		endTime = ""
		detail = nlFormatDate(t) + " - " + nlFormatDate(now)
	}

	return
}

// nl2dslExtractKey 提取关键词。
func nl2dslExtractKey(text string) string {
	// 引号内文本优先级最高
	if m := nlQuotedPattern.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}

	// 清理时间描述、级别关键词
	cleaned := nlCleanupTimeRef.ReplaceAllString(text, "")
	cleaned = nlCleanupLevel.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

// nl2dslDetectIntent 判断意图类型。
func nl2dslDetectIntent(level, startTime, endTime, key string) string {
	switch {
	case level != "" && key != "":
		return INTENT_ERROR_SEARCH
	case level != "":
		return INTENT_ERROR_SEARCH
	case startTime != "" || endTime != "":
		return INTENT_TIME_RANGE
	case key != "":
		return INTENT_KEYWORD
	default:
		return INTENT_GENERAL
	}
}

// nl2dslFormatConditions 格式化 DSL 条件表达式。
func nl2dslFormatConditions(level, key, date, startTime, endTime string) string {
	var parts []string
	if level != "" {
		parts = append(parts, "级别="+level)
	}
	if key != "" {
		parts = append(parts, `关键词="`+key+`"`)
	}
	if date != "" {
		parts = append(parts, "日期="+date)
	}
	if startTime != "" && endTime != "" {
		parts = append(parts, "时间="+startTime+"~"+endTime)
	}
	if len(parts) == 0 {
		return "无条件（全量）"
	}
	return strings.Join(parts, " AND ")
}

// 工具函数

func nlFormatDate(d time.Time) string {
	return d.Format("20060102")
}

func nlFormatTime(d time.Time) string {
	return d.Format("15:04:05")
}

func nlParseInt(s string) int {
	n := 0
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		}
	}
	if n <= 0 {
		n = 1
	}
	return n
}
