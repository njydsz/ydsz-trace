package parser

import (
	"strings"
	"testing"
)

// TestMergeMultiLine_JavaStack Java 堆栈应合并为一个条目。
func TestMergeMultiLine_JavaStack(t *testing.T) {
	lines := []string{
		`2026-11-03 10:00:00 ERROR [main] com.example.Service - request failed`,
		`    at com.example.Service.handle(Service.java:42)`,
		`    at com.example.Controller.dispatch(Controller.java:17)`,
		`    ... 36 more`,
		`Caused by: java.lang.NullPointerException: user is null`,
		`    at com.example.Service.user(Service.java:88)`,
	}

	entries := MergeMultiLine(lines)
	if len(entries) != 1 {
		t.Fatalf("应合并为 1 个条目, got %d", len(entries))
	}
	if entries[0].Kind != KindJavaStack {
		t.Errorf("kind = %q want %q", entries[0].Kind, KindJavaStack)
	}
	if entries[0].LineCount() != 6 {
		t.Errorf("line count = %d want 6", entries[0].LineCount())
	}
	if !strings.Contains(entries[0].MergedMessage, "Caused by:") {
		t.Error("合并消息应包含根因链")
	}
}

// TestMergeMultiLine_PythonTraceback Python traceback 应合并。
func TestMergeMultiLine_PythonTraceback(t *testing.T) {
	lines := []string{
		`2026-11-03 10:00:05 ERROR app - unhandled exception`,
		`Traceback (most recent call last):`,
		`  File "/app/main.py", line 12, in <module>`,
		`    run()`,
		`  File "/app/runner.py", line 30, in run`,
		`    raise ValueError("bad input")`,
		`ValueError: bad input`,
	}

	entries := MergeMultiLine(lines)
	if len(entries) != 1 {
		t.Fatalf("应合并为 1 个条目, got %d", len(entries))
	}
	if entries[0].Kind != KindPythonTraceback {
		t.Errorf("kind = %q want %q", entries[0].Kind, KindPythonTraceback)
	}
}

// TestMergeMultiLine_GoPanic Go panic 栈应合并。
func TestMergeMultiLine_GoPanic(t *testing.T) {
	lines := []string{
		`2026-11-03 10:00:10 ERROR server - panic recovered`,
		`goroutine 1 [running]:`,
		`main.main()`,
		`	/app/cmd/main.go:25 +0x1a`,
		`created by runtime.main in goroutine 1`,
	}

	entries := MergeMultiLine(lines)
	if len(entries) != 1 {
		t.Fatalf("应合并为 1 个条目, got %d", len(entries))
	}
	if entries[0].Kind != KindGoPanic {
		t.Errorf("kind = %q want %q", entries[0].Kind, KindGoPanic)
	}
}

// TestMergeMultiLine_MultipleEntries 多条独立日志各自成条目。
func TestMergeMultiLine_MultipleEntries(t *testing.T) {
	lines := []string{
		`2026-11-03 10:00:00 INFO started`,
		`2026-11-03 10:00:01 ERROR failed`,
		`    at a.b.C.d(C.java:1)`,
		`2026-11-03 10:00:02 INFO recovered`,
	}

	entries := MergeMultiLine(lines)
	if len(entries) != 3 {
		t.Fatalf("应为 3 个条目, got %d", len(entries))
	}
	if entries[0].Kind != KindSingle {
		t.Errorf("entry0 kind = %q want single", entries[0].Kind)
	}
	if entries[1].Kind != KindJavaStack {
		t.Errorf("entry1 kind = %q want java-stack", entries[1].Kind)
	}
	if entries[2].Kind != KindSingle {
		t.Errorf("entry2 kind = %q want single", entries[2].Kind)
	}
}

// TestMergeMultiLine_NoMergeForStructurelessText 无结构文本不合并。
func TestMergeMultiLine_NoMergeForStructurelessText(t *testing.T) {
	lines := []string{
		`some plain text`,
		`    indented continuation`,
		`    another indented line`,
	}

	entries := MergeMultiLine(lines)
	if len(entries) != 3 {
		t.Fatalf("无时间戳/级别的文本不应合并, got %d 条", len(entries))
	}
	for i, e := range entries {
		if e.Kind != KindSingle {
			t.Errorf("entry %d kind = %q want single", i, e.Kind)
		}
	}
}

// TestMergeMultiLine_EmptyLines 空行单独成条目，不吞并相邻内容。
func TestMergeMultiLine_EmptyLines(t *testing.T) {
	lines := []string{
		`2026-11-03 10:00:00 INFO a`,
		``,
		`2026-11-03 10:00:01 INFO b`,
	}

	entries := MergeMultiLine(lines)
	if len(entries) != 3 {
		t.Fatalf("应为 3 个条目, got %d", len(entries))
	}
	if entries[1].Kind != KindSingle {
		t.Errorf("空行应为 single, got %q", entries[1].Kind)
	}
}

// TestMergeMultiLine_BlankInput 空输入返回 nil。
func TestMergeMultiLine_BlankInput(t *testing.T) {
	if got := MergeMultiLine(nil); got != nil {
		t.Errorf("nil 输入应返回 nil, got %v", got)
	}
	if got := MergeMultiLine([]string{}); got != nil {
		t.Errorf("空切片应返回 nil, got %v", got)
	}
}

// TestMergeMultiLine_LevelPrefixNoTimestamp 仅级别无时间戳也能作为主行。
func TestMergeMultiLine_LevelPrefixNoTimestamp(t *testing.T) {
	lines := []string{
		`ERROR something broke`,
		`    at x.y.Z.a(Z.java:1)`,
	}

	entries := MergeMultiLine(lines)
	if len(entries) != 1 {
		t.Fatalf("应合并为 1 条, got %d", len(entries))
	}
}

// TestMergeMultiLine_BracketLevel 方括号级别格式。
func TestMergeMultiLine_BracketLevel(t *testing.T) {
	lines := []string{
		`[2026-11-03 10:00:00] [ERROR] boom`,
		`    at x.y.Z.a(Z.java:1)`,
	}

	entries := MergeMultiLine(lines)
	// 主行以 '[' 开头但内嵌时间戳，looksLikePrimary 走 hasLeadingTimestamp 失败，
	// 此时应作为单行输出，从属行也不应被吞并到无结构文本中。
	if len(entries) == 0 {
		t.Fatal("不应为空")
	}
	// 至少第一行必须是 single（未误判为从属）
	if entries[0].Kind != KindSingle {
		t.Errorf("首行应为 single, got %q", entries[0].Kind)
	}
}

// TestLooksLikePrimary 主行判定表驱动测试。
func TestLooksLikePrimary(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{`2026-11-03 10:00:00 INFO x`, true},
		{`2026-11-03T10:00:00Z INFO x`, true},
		{`1700000000 INFO x`, true},
		{`ERROR x`, true},
		{`[INFO] x`, true},
		{`{"level":"info"}`, true},
		{`    at a.b.C.d(C.java:1)`, false},
		{`plain text`, false},
		{``, false},
		{`INFORMATIONAL message`, false}, // 前缀匹配但不能算 INFO
	}

	for _, c := range cases {
		if got := looksLikePrimary(c.line); got != c.want {
			t.Errorf("looksLikePrimary(%q) = %v want %v", c.line, got, c.want)
		}
	}
}

// TestClassifyContinuation 从属行分类表驱动测试。
func TestClassifyContinuation(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"    at a.b.C.d(C.java:1)", KindJavaStack},
		{"    ... 3 more", KindJavaStack},
		{"Caused by: java.lang.Exception", KindJavaStack},
		{"Suppressed: java.lang.Exception", KindJavaStack},
		{`  File "/app/x.py", line 1, in f`, KindPythonTraceback},
		{"    ^", KindPythonTraceback},
		{"goroutine 1 [running]:", KindGoPanic},
		{"	/app/main.go:25 +0x1a", KindGoPanic},
		{"created by runtime.main", KindGoPanic},
		{"    some indented text", KindGenericIndent},
		{"INFO normal line", KindSingle},
		{"", KindSingle},
	}

	for _, c := range cases {
		if got := classifyContinuation(c.line); got != c.want {
			t.Errorf("classifyContinuation(%q) = %q want %q", c.line, got, c.want)
		}
	}
}
