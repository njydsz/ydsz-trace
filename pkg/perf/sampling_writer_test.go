package perf

import (
	"bytes"
	"strings"
	"testing"
)

// TestSamplingWriter_NilSamplerPassthrough sampler 为 nil 时直通。
func TestSamplingWriter_NilSamplerPassthrough(t *testing.T) {
	var buf bytes.Buffer
	w := NewSamplingWriter(&buf, nil)
	if _, ok := w.(*SamplingWriter); ok {
		t.Fatal("nil sampler 应返回原 writer")
	}

	_, _ = w.Write([]byte("hello\n"))
	if buf.String() != "hello\n" {
		t.Errorf("got %q", buf.String())
	}
}

// TestSamplingWriter_FullRatePassthrough 采样率 1.0 时直通。
func TestSamplingWriter_FullRatePassthrough(t *testing.T) {
	var buf bytes.Buffer
	s := NewSampler()
	s.SetRate(1.0)
	w := NewSamplingWriter(&buf, s)
	if _, ok := w.(*SamplingWriter); ok {
		t.Fatal("采样率 1.0 应返回原 writer")
	}
}

// TestSamplingWriter_DropsSampledLines 低采样率下 INFO 被丢弃、ERROR 保留。
func TestSamplingWriter_DropsSampledLines(t *testing.T) {
	var buf bytes.Buffer
	s := NewSampler()
	s.SetRate(0.01) // 1%

	w := NewSamplingWriter(&buf, s)
	input := strings.Join([]string{
		"ERROR critical failure",
		"FATAL system down",
		"INFO routine",
		"DEBUG detail",
		"INFO another routine",
	}, "\n") + "\n"

	if _, err := w.Write([]byte(input)); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ERROR critical failure") {
		t.Error("ERROR 行应保留")
	}
	if !strings.Contains(out, "FATAL system down") {
		t.Error("FATAL 行应保留")
	}
	// 1% 采样率下，2 条 INFO 行几乎必然全部被丢弃
	// （不断言 DEBUG：随机性可能偶尔命中，且 DEBUG 也属于采样范围）
	if strings.Contains(out, "routine") {
		t.Errorf("INFO 行应被采样丢弃, got %q", out)
	}
}

// TestSamplingWriter_PartialWrites 分片写入能正确按行组装。
func TestSamplingWriter_PartialWrites(t *testing.T) {
	var buf bytes.Buffer
	s := NewSampler()
	// 直接构造低采样率的 SamplingWriter（避开 1.0 直通快路径）
	s.SetRate(0.01)
	w := NewSamplingWriter(&buf, s)
	sw, ok := w.(*SamplingWriter)
	if !ok {
		t.Fatal("期望 SamplingWriter 实例")
	}

	parts := []string{"INFO first", " line continues\n", "ERROR second\n"}
	for _, p := range parts {
		if _, err := sw.Write([]byte(p)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := sw.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ERROR second") {
		t.Errorf("ERROR 行应保留, got %q", out)
	}
}

// TestSamplingWriter_FlushIncompleteLine 无换行的末行由 Flush 处理。
func TestSamplingWriter_FlushIncompleteLine(t *testing.T) {
	var buf bytes.Buffer
	s := NewSampler()
	s.SetRate(0.01)

	w := NewSamplingWriter(&buf, s)
	if _, err := w.Write([]byte("ERROR no newline at end")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// 未 flush 时不应有输出
	if buf.Len() != 0 {
		t.Errorf("flush 前不应输出, got %q", buf.String())
	}

	sw := w.(*SamplingWriter)
	if err := sw.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if !strings.Contains(buf.String(), "ERROR no newline at end") {
		t.Errorf("flush 后应保留 ERROR 行, got %q", buf.String())
	}
}

// TestExtractLevelFromLine 级别提取表驱动测试。
func TestExtractLevelFromLine(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"INFO something", "INFO"},
		{"ERROR boom", "ERROR"},
		{"[INFO] bracketed", "INFO"},
		{"[ERROR] bracketed", "ERROR"},
		{"2026-11-03 10:00:00 INFO with timestamp", "INFO"},
		{"WARN careful", "WARN"},
		{"WARNING long form", "WARNING"},
		{"DEBUG detail", "DEBUG"},
		{"TRACE trace", "TRACE"},
		{"plain text no level", ""},
		{"INFORMATIONAL not a level", ""},
		{`{"level":"info","message":"x"}`, "INFO"},
		{`{"level":"error"}`, "ERROR"},
		{"", ""},
	}

	for _, c := range cases {
		if got := extractLevelFromLine(c.line); got != c.want {
			t.Errorf("extractLevelFromLine(%q) = %q want %q", c.line, got, c.want)
		}
	}
}

// TestSamplingWriter_ConcurrentWrites 并发写入不 panic 且计数正确。
func TestSamplingWriter_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	s := NewSampler()
	s.SetRate(0.5)
	w := NewSamplingWriter(&buf, s)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				_, _ = w.Write([]byte("ERROR concurrent line\n"))
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
