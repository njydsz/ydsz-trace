package source

import (
	"strings"
	"testing"
)

func TestMultilineMerger(t *testing.T) {
	merger := NewMultilineMerger()

	lines := []string{
		"2026-01-01 10:00:00 ERROR 用户登录失败",
		"    at com.example.UserService.login(UserService.java:42)",
		"    at com.example.AuthFilter.doFilter(AuthFilter.java:28)",
		"2026-01-01 10:00:01 INFO 请求处理完成",
	}

	var events []string
	for _, line := range lines {
		event, done := merger.Feed(line)
		if done && event != "" {
			events = append(events, event)
		}
	}
	if tail := merger.Flush(); tail != "" {
		events = append(events, tail)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// 第一个事件应包含 3 行（起始行 + 2 行堆栈）
	if !strings.Contains(events[0], "UserService.login") {
		t.Errorf("first event should contain stack trace, got: %s", events[0])
	}
	if !strings.Contains(events[0], "AuthFilter.doFilter") {
		t.Errorf("first event should contain second stack line, got: %s", events[0])
	}

	// 第二个事件应只有 1 行
	if strings.Contains(events[1], "at ") {
		t.Errorf("second event should not contain stack trace, got: %s", events[1])
	}
}

func TestMultilineMergerFlush(t *testing.T) {
	merger := NewMultilineMerger()

	// 只输入起始行 + 续行，不输入新的起始行
	merger.Feed("2026-01-01 10:00:00 ERROR something")
	merger.Feed("  stack line 1")
	merger.Feed("  stack line 2")

	// Flush 应输出累积的事件
	event := merger.Flush()
	if event == "" {
		t.Fatal("expected event after flush, got empty")
	}
	if !strings.Contains(event, "stack line 1") {
		t.Errorf("event should contain stack line, got: %s", event)
	}
	if !strings.Contains(event, "stack line 2") {
		t.Errorf("event should contain second stack line, got: %s", event)
	}
}

func TestMultilineMergerMaxLines(t *testing.T) {
	merger := NewMultilineMerger()
	merger.Feed("2026-01-01 10:00:00 ERROR start")

	// 输入超过 maxMergedLines 的续行
	for i := 0; i < maxMergedLines+10; i++ {
		merger.Feed("  stack line")
	}

	event := merger.Flush()
	if event == "" {
		t.Fatal("expected event after flush")
	}
	// 验证行数不超过限制（+1 是起始行）
	lineCount := strings.Count(event, "\n") + 1
	if lineCount > maxMergedLines+2 {
		t.Errorf("expected at most %d lines, got %d", maxMergedLines+2, lineCount)
	}
}

func TestMergeLines(t *testing.T) {
	lines := []string{
		"2026-01-01 10:00:00 ERROR 数据库连接失败",
		"    at com.example.DB.connect(DB.java:15)",
		"2026-01-01 10:00:01 WARN 重试中...",
		"    at com.example.DB.retry(DB.java:20)",
		"2026-01-01 10:00:02 INFO 连接成功",
	}

	events := MergeLines(lines)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// 验证每个事件的行数
	if strings.Count(events[0], "\n") != 1 {
		t.Errorf("event 0 should have 2 lines, got %d", strings.Count(events[0], "\n")+1)
	}
	if strings.Count(events[1], "\n") != 1 {
		t.Errorf("event 1 should have 2 lines, got %d", strings.Count(events[1], "\n")+1)
	}
	if strings.Count(events[2], "\n") != 0 {
		t.Errorf("event 2 should have 1 line, got %d", strings.Count(events[2], "\n")+1)
	}
}

func TestMultilineMergerEmptyInput(t *testing.T) {
	merger := NewMultilineMerger()

	event, done := merger.Feed("")
	if event != "" || done {
		t.Error("empty line should not produce event")
	}

	if tail := merger.Flush(); tail != "" {
		t.Error("flush on empty merger should return empty")
	}
}
