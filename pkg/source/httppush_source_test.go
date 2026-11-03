package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestJournalLineToText 单条 journal JSON 格式化。
func TestJournalLineToText(t *testing.T) {
	raw := `{"MESSAGE":"service started","PRIORITY":"6","_HOSTNAME":"node1","SYSLOG_IDENTIFIER":"app","_PID":"1234"}`
	got := journalLineToText([]byte(raw))
	want := "INFO node1 app[1234]: service started"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestJournalLineToText_ErrorPriority 错误级别映射。
func TestJournalLineToText_ErrorPriority(t *testing.T) {
	raw := `{"MESSAGE":"boom","PRIORITY":"3"}`
	if got := journalLineToText([]byte(raw)); got != "ERROR boom" {
		t.Errorf("got %q", got)
	}
}

// TestJournalLineToText_InvalidJSON 非 JSON 原样返回。
func TestJournalLineToText_InvalidJSON(t *testing.T) {
	raw := `journalctl: some warning`
	if got := journalLineToText([]byte(raw)); got != raw {
		t.Errorf("got %q want %q", got, raw)
	}
}

// TestJournalLineToText_EmptyMessage 空 MESSAGE 返回空串。
func TestJournalLineToText_EmptyMessage(t *testing.T) {
	if got := journalLineToText([]byte(`{"MESSAGE":"","PRIORITY":"6"}`)); got != "" {
		t.Errorf("got %q want empty", got)
	}
}

// TestPriorityName 优先级映射表。
func TestPriorityName(t *testing.T) {
	cases := map[string]string{
		"0": "FATAL", "1": "FATAL", "2": "FATAL",
		"3": "ERROR", "4": "WARN", "5": "INFO", "6": "INFO", "7": "DEBUG",
		"9": "", "": "",
	}
	for in, want := range cases {
		if got := priorityName(in); got != want {
			t.Errorf("priorityName(%q) = %q want %q", in, got, want)
		}
	}
}

// TestJournalReader_Read 读取多条记录并转为文本行。
func TestJournalReader_Read(t *testing.T) {
	input := strings.Join([]string{
		`{"MESSAGE":"one","PRIORITY":"6"}`,
		`{"MESSAGE":"two","PRIORITY":"3"}`,
		``,
	}, "\n")

	r := newJournalReader(strings.NewReader(input))
	var out strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("read: %v", err)
		}
	}

	text := out.String()
	if !strings.Contains(text, "INFO one") {
		t.Errorf("missing first line: %q", text)
	}
	if !strings.Contains(text, "ERROR two") {
		t.Errorf("missing second line: %q", text)
	}
}

// TestHTTPPushSource_TextPush 纯文本推送入库后可检索。
func TestHTTPPushSource_TextPush(t *testing.T) {
	hp := NewHTTPPushSource(FactoryConfig{Options: map[string]string{}})

	body := "INFO first line\nERROR second line\n"
	req := httptest.NewRequest(http.MethodPost, hp.path, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	hp.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var sb strings.Builder
	n, err := hp.Read(context.Background(), "", ScanConfig{}, &sb)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n == 0 {
		t.Fatal("应读到推送的日志")
	}
	if !strings.Contains(sb.String(), "ERROR second line") {
		t.Errorf("missing content: %q", sb.String())
	}
}

// TestHTTPPushSource_JSONArrayPush JSON 数组推送。
func TestHTTPPushSource_JSONArrayPush(t *testing.T) {
	hp := NewHTTPPushSource(FactoryConfig{Options: map[string]string{}})

	body := `[{"message":"alpha","level":"INFO"},{"message":"beta failed","level":"ERROR"}]`
	req := httptest.NewRequest(http.MethodPost, hp.path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	hp.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var sb strings.Builder
	if _, err := hp.Read(context.Background(), "", ScanConfig{}, &sb); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(sb.String(), "ERROR beta failed") {
		t.Errorf("missing formatted entry: %q", sb.String())
	}
}

// TestHTTPPushSource_WrongPath 错误路径 404。
func TestHTTPPushSource_WrongPath(t *testing.T) {
	hp := NewHTTPPushSource(FactoryConfig{Options: map[string]string{}})
	req := httptest.NewRequest(http.MethodPost, "/wrong", strings.NewReader("x"))
	rec := httptest.NewRecorder()
	hp.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d want 404", rec.Code)
	}
}

// TestHTTPPushSource_WrongMethod 错误方法 405。
func TestHTTPPushSource_WrongMethod(t *testing.T) {
	hp := NewHTTPPushSource(FactoryConfig{Options: map[string]string{}})
	req := httptest.NewRequest(http.MethodGet, hp.path, nil)
	rec := httptest.NewRecorder()
	hp.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d want 405", rec.Code)
	}
}

// TestHTTPPushSource_BoundedBuffer 超出容量丢弃最旧数据。
func TestHTTPPushSource_BoundedBuffer(t *testing.T) {
	hp := NewHTTPPushSource(FactoryConfig{Options: map[string]string{"max_buffer": "3"}})

	for i := 0; i < 5; i++ {
		hp.append("line")
	}

	hp.mu.Lock()
	got := len(hp.buffer)
	hp.mu.Unlock()
	if got != 3 {
		t.Errorf("buffer len = %d want 3", got)
	}
}

// TestHTTPPushSource_TailReceivesPush Tail 能收到推送。
func TestHTTPPushSource_TailReceivesPush(t *testing.T) {
	hp := NewHTTPPushSource(FactoryConfig{Options: map[string]string{}})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got := make(chan string, 1)
	go func() {
		_ = hp.Tail(ctx, "", TailConfig{}, func(line string) error {
			select {
			case got <- line:
			default:
			}
			return nil
		})
	}()

	// 等待 Tail 就绪后推送
	time.Sleep(300 * time.Millisecond)
	body := "ERROR tail-received"
	req := httptest.NewRequest(http.MethodPost, hp.path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	hp.ServeHTTP(rec, req)

	select {
	case line := <-got:
		if !strings.Contains(line, "tail-received") {
			t.Errorf("unexpected line: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Tail 未收到推送日志")
	}
}

// TestHTTPPushSource_StopIdempotent 重复停止不 panic。
func TestHTTPPushSource_StopIdempotent(t *testing.T) {
	hp := NewHTTPPushSource(FactoryConfig{Options: map[string]string{}})
	hp.Stop()
	hp.Stop()
}

// TestSyslogSource_BoundedBuffer syslog 缓冲有界。
func TestSyslogSource_BoundedBuffer(t *testing.T) {
	ss := NewSyslogSource(FactoryConfig{Options: map[string]string{"max_buffer": "2"}})
	for i := 0; i < 5; i++ {
		ss.append("x")
	}

	ss.mu.Lock()
	got := len(ss.buffer)
	ss.mu.Unlock()
	if got != 2 {
		t.Errorf("buffer len = %d want 2", got)
	}
}

// TestSyslogSource_ReadEmpty 空缓冲读取返回 EOF 语义（0 字节）。
func TestSyslogSource_ReadEmpty(t *testing.T) {
	ss := NewSyslogSource(FactoryConfig{Options: map[string]string{}})
	var sb strings.Builder
	n, err := ss.Read(context.Background(), "", ScanConfig{}, &sb)
	if err != nil {
		t.Fatalf("read empty: %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d want 0", n)
	}
}

// TestSyslogSource_IntParse 配置解析辅助。
func TestSyslogSource_IntParse(t *testing.T) {
	if n, err := parseInt("123"); err != nil || n != 123 {
		t.Errorf("parseInt(123) = %d, %v", n, err)
	}
	if _, err := parseInt("12a"); err == nil {
		t.Error("非法输入应返回错误")
	}
	if _, err := parseInt(""); err == nil {
		t.Error("空输入应返回错误")
	}
}

// TestRegisteredTypes_IncludeNewSources 新增源类型已注册。
func TestRegisteredTypes_IncludeNewSources(t *testing.T) {
	types := RegisteredTypes()
	found := map[SourceType]bool{}
	for _, t := range types {
		found[t] = true
	}
	for _, want := range []SourceType{SourceTypeFile, SourceTypeJournald, SourceTypeSyslog, SourceTypeHTTPPush} {
		if !found[want] {
			t.Errorf("未注册源类型: %s", want)
		}
	}
}
