package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newCtx() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	return c, w
}

func TestSuccess(t *testing.T) {
	c, w := newCtx()
	Success(c, "ok", map[string]string{"k": "v"})

	if w.Code != http.StatusOK {
		t.Fatalf("状态码错误: got %d want %d", w.Code, http.StatusOK)
	}
	var body Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体非合法 JSON: %v", err)
	}
	if body.Code != CodeSuccess {
		t.Fatalf("code 错误: got %q", body.Code)
	}
	if body.Message != "ok" {
		t.Fatalf("message 错误: got %q", body.Message)
	}
	if body.Data == nil {
		t.Fatalf("data 不应为空")
	}
}

func TestSuccess_DefaultMessage(t *testing.T) {
	c, w := newCtx()
	Success(c, "", nil)
	var body Response
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Message != "操作成功" {
		t.Fatalf("默认消息错误: got %q", body.Message)
	}
}

func TestError(t *testing.T) {
	c, w := newCtx()
	Error(c, http.StatusBadRequest, "bad")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码错误: got %d want %d", w.Code, http.StatusBadRequest)
	}
	var body Response
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != CodeBadRequest {
		t.Fatalf("code 错误: got %q", body.Code)
	}
	if body.Message != "bad" {
		t.Fatalf("message 错误: got %q", body.Message)
	}
}

func TestFail(t *testing.T) {
	c, w := newCtx()
	Fail(c, CodeForbidden, "no")
	if w.Code != http.StatusForbidden {
		t.Fatalf("状态码错误: got %d want %d", w.Code, http.StatusForbidden)
	}
	var body Response
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != CodeForbidden {
		t.Fatalf("code 错误: got %q", body.Code)
	}
}

func TestPaginated(t *testing.T) {
	c, w := newCtx()
	Paginated(c, "", []int{1, 2}, 2, 1, 10)
	var body Response
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != CodeSuccess {
		t.Fatalf("code 错误: got %q", body.Code)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data 结构错误: %T", body.Data)
	}
	if data["total"] != float64(2) {
		t.Fatalf("total 错误: %v", data["total"])
	}
	if data["totalPage"] != float64(1) {
		t.Fatalf("totalPage 错误: %v", data["totalPage"])
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	cases := map[int]ResponseCode{
		http.StatusOK:           CodeSuccess,
		http.StatusBadRequest:   CodeBadRequest,
		http.StatusUnauthorized: CodeUnauthorized,
		http.StatusForbidden:    CodeForbidden,
		http.StatusNotFound:     CodeNotFound,
	}
	for httpStatus, wantCode := range cases {
		if got := httpStatusToCode(httpStatus); got != wantCode {
			t.Fatalf("httpStatusToCode(%d) = %q want %q", httpStatus, got, wantCode)
		}
		if got := codeToHTTPStatus(wantCode); got != httpStatus {
			t.Fatalf("codeToHTTPStatus(%q) = %d want %d", wantCode, got, httpStatus)
		}
	}
}
