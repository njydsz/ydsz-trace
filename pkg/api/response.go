// Package api 提供统一的 API 响应封装
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ResponseCode 响应码定义
type ResponseCode string

const (
	CodeSuccess      ResponseCode = "200"
	CodeBadRequest   ResponseCode = "400"
	CodeUnauthorized ResponseCode = "401"
	CodeForbidden    ResponseCode = "403"
	CodeNotFound     ResponseCode = "404"
	CodeServerError  ResponseCode = "500"
	CodeTimeout      ResponseCode = "504"
)

// Response 统一响应结构
type Response struct {
	Code    ResponseCode `json:"code"`
	Message string       `json:"message"`
	Data    interface{}  `json:"data,omitempty"`
	TraceID string       `json:"traceId,omitempty"`
}

// Success 成功响应
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

// Fail 错误响应
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

// FailWithData 带数据的错误响应
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

// Error 失败响应（兼容老接口格式）
func Error(c *gin.Context, httpStatus int, msg string) {
	c.JSON(httpStatus, Response{
		Code:    httpStatusToCode(httpStatus),
		Message: msg,
		Data:    nil,
		TraceID: getTraceID(c),
	})
}

// Paginated 分页响应
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

// getTraceID 从 context 获取 TraceID
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
