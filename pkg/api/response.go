// Package api 提供统一的 API 响应封装。
//
// 响应格式遵循统一规范：
//
//	{
//	  "code": "200",       // 业务状态码，字符串类型
//	  "message": "success", // 提示信息
//	  "data": {},           // 业务数据（可选）
//	  "traceId": "xxx"      // 链路追踪 ID（可选）
//	}
//
// 业务码与 HTTP 状态码通过 codeToHTTPStatus / httpStatusToCode 双向映射。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ResponseCode 业务响应码，字符串类型便于前端精确匹配。
//
// 命名规则：Code + HTTP 语义，值与 HTTP 状态码保持一致，方便双向转换。
type ResponseCode string

const (
	// CodeSuccess 请求成功
	CodeSuccess ResponseCode = "200"
	// CodeBadRequest 请求参数错误
	CodeBadRequest ResponseCode = "400"
	// CodeUnauthorized 未认证（未登录或 token 失效）
	CodeUnauthorized ResponseCode = "401"
	// CodeForbidden 权限不足
	CodeForbidden ResponseCode = "403"
	// CodeNotFound 资源不存在
	CodeNotFound ResponseCode = "404"
	// CodeServerError 服务内部错误
	CodeServerError ResponseCode = "500"
	// CodeTimeout 网关超时
	CodeTimeout ResponseCode = "504"
)

// Response 统一 API 响应结构体。
//
// 所有遵循规范的 handler 应通过 Success / Fail / Paginated 输出该结构，
// 确保前端处理逻辑一致。
type Response struct {
	// Code 业务状态码，详见 ResponseCode 定义
	Code ResponseCode `json:"code"`
	// Message 提示信息，面向终端用户
	Message string `json:"message"`
	// Data 业务数据负载，成功时返回，失败时为 nil 或错误详情
	Data interface{} `json:"data,omitempty"`
	// TraceID 链路追踪标识，用于跨服务日志关联
	TraceID string `json:"traceId,omitempty"`
}

// Success 返回成功响应（HTTP 200）。
//
// 参数：
//   - c: gin 上下文
//   - msg: 提示信息为空时默认 "操作成功"
//   - data: 业务数据负载，可 nil
func Success(c *gin.Context, msg string, data interface{}) {
	if msg == "" {
		msg = "操作成功"
	}
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: msg,
		Data:    data,
		TraceID: getTraceID(c),
	})
}

// Fail 返回错误响应，HTTP 状态码由业务码自动映射。
//
// 参数：
//   - c: gin 上下文
//   - code: 业务错误码（如 CodeBadRequest）
//   - msg: 错误描述为空时默认 "操作失败"
func Fail(c *gin.Context, code ResponseCode, msg string) {
	if msg == "" {
		msg = "操作失败"
	}
	httpStatus := codeToHTTPStatus(code)
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: msg,
		Data:    nil,
		TraceID: getTraceID(c),
	})
}

// FailWithData 返回带错误详情的错误响应。
//
// 适用于参数校验失败等需要返回结构化错误明细的场景。
func FailWithData(c *gin.Context, code ResponseCode, msg string, data interface{}) {
	if msg == "" {
		msg = "操作失败"
	}
	httpStatus := codeToHTTPStatus(code)
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: msg,
		Data:    data,
		TraceID: getTraceID(c),
	})
}

// Error 兼容以 HTTP 状态码为入参的旧接口格式。
//
// 新代码应优先使用 Fail / FailWithData，以业务码驱动响应。
func Error(c *gin.Context, httpStatus int, msg string) {
	c.JSON(httpStatus, Response{
		Code:    httpStatusToCode(httpStatus),
		Message: msg,
		Data:    nil,
		TraceID: getTraceID(c),
	})
}

// Paginated 返回标准分页响应。
//
// data 结构：
//
//	{
//	  "list": [],        // 当前页数据
//	  "total": 100,      // 总记录数
//	  "pageNo": 1,       // 当前页码（从 1 开始）
//	  "pageSize": 10,    // 每页条数
//	  "totalPage": 10    // 总页数（向上取整）
//	}
func Paginated(c *gin.Context, msg string, list interface{}, total, pageNum, pageSize int) {
	if msg == "" {
		msg = "查询成功"
	}
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: msg,
		Data: map[string]interface{}{
			"list":      list,
			"total":     total,
			"pageNo":    pageNum,
			"pageSize":  pageSize,
			"totalPage": (total + pageSize - 1) / pageSize,
		},
		TraceID: getTraceID(c),
	})
}

// getTraceID 从 gin 上下文中提取链路追踪 ID。
//
// 如果中间件未注入或类型不匹配，返回空字符串（调用方应能处理空值）。
func getTraceID(c *gin.Context) string {
	traceID, exists := c.Get("traceId")
	if exists {
		if s, ok := traceID.(string); ok {
			return s
		}
	}
	return ""
}

// codeToHTTPStatus 响应码转 HTTP 状态码
func codeToHTTPStatus(code ResponseCode) int {
	switch code {
	case CodeSuccess:
		return http.StatusOK
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeServerError:
		return http.StatusInternalServerError
	case CodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// httpStatusToCode HTTP 状态码转响应码
func httpStatusToCode(status int) ResponseCode {
	switch status {
	case http.StatusOK:
		return CodeSuccess
	case http.StatusBadRequest:
		return CodeBadRequest
	case http.StatusUnauthorized:
		return CodeUnauthorized
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeNotFound
	default:
		return CodeServerError
	}
}
