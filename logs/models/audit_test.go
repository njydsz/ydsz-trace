// Package models 审计日志测试。
package models

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestDB 创建临时 SQLite 数据库用于测试，测试结束后自动关闭。
func setupTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")
	cfg := &SQLiteConfig{FilePath: dbPath}
	if err := InitDB(cfg); err != nil {
		t.Fatalf("初始化测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if DB != nil {
			_ = DB.Close()
		}
	})
}

// TestAppendAuditLog 测试写入审计日志。
func TestAppendAuditLog(t *testing.T) {
	setupTestDB(t)

	detail := map[string]interface{}{
		"itemId":    int64(1),
		"matchCount": 42,
	}
	err := AppendAuditLog("admin", "search", "/logs/search", detail, "127.0.0.1", "success")
	if err != nil {
		t.Fatalf("AppendAuditLog failed: %v", err)
	}

	// 写入第二条（测试不同 action）
	err = AppendAuditLog("tester", "export", "/logs/query", map[string]interface{}{
		"clientId": int64(0),
	}, "192.168.1.1", "success")
	if err != nil {
		t.Fatalf("AppendAuditLog (2nd) failed: %v", err)
	}

	// 写入第三条（空 detail）
	err = AppendAuditLog("", "register", "/client/register", nil, "10.0.0.1", "success")
	if err != nil {
		t.Fatalf("AppendAuditLog (3rd) failed: %v", err)
	}
}

// TestQueryAuditLogs 测试分页查询审计日志。
func TestQueryAuditLogsByUsername(t *testing.T) {
	setupTestDB(t)

	// 插入测试数据
	for i := 0; i < 5; i++ {
		_ = AppendAuditLog("admin", "search", "/logs/search", map[string]interface{}{"index": i}, "127.0.0.1", "success")
	}
	for i := 0; i < 3; i++ {
		_ = AppendAuditLog("viewer", "export", "/logs/query", map[string]interface{}{"index": i}, "192.168.1.1", "success")
	}

	// 查询全部
	logs, total, err := QueryAuditLogs(1, 20, "", "")
	if err != nil {
		t.Fatalf("QueryAuditLogs failed: %v", err)
	}
	if total != 8 {
		t.Errorf("期望总数 8，实际 %d", total)
	}
	if len(logs) != 8 {
		t.Errorf("期望返回 8 条，实际 %d 条", len(logs))
	}

	// 按 username 过滤
	logs, total, err = QueryAuditLogs(1, 20, "admin", "")
	if err != nil {
		t.Fatalf("QueryAuditLogs (filter username) failed: %v", err)
	}
	if total != 5 {
		t.Errorf("期望 admin 总数 5，实际 %d", total)
	}
	for _, l := range logs {
		if l.Username != "admin" {
			t.Errorf("期望 username=admin，实际 %s", l.Username)
		}
	}

	// 按 action 过滤
	logs, total, err = QueryAuditLogs(1, 20, "", "export")
	if err != nil {
		t.Fatalf("QueryAuditLogs (filter action) failed: %v", err)
	}
	if total != 3 {
		t.Errorf("期望 export 总数 3，实际 %d", total)
	}

	// 组合过滤
	logs, total, err = QueryAuditLogs(1, 20, "viewer", "export")
	if err != nil {
		t.Fatalf("QueryAuditLogs (combined filter) failed: %v", err)
	}
	if total != 3 {
		t.Errorf("期望 viewer+export 总数 3，实际 %d", total)
	}

	// 分页
	logs, total, err = QueryAuditLogs(1, 3, "", "")
	if err != nil {
		t.Fatalf("QueryAuditLogs (paged) failed: %v", err)
	}
	if total != 8 {
		t.Errorf("分页查询总数应仍为 8，实际 %d", total)
	}
	if len(logs) != 3 {
		t.Errorf("第一页应返回 3 条，实际 %d", len(logs))
	}
}

// TestQueryAuditLogsPaging 测试分页边界。
func TestQueryAuditLogsPaging(t *testing.T) {
	setupTestDB(t)

	// 写入 5 条
	for i := 0; i < 5; i++ {
		_ = AppendAuditLog("admin", "search", "/logs/search", nil, "127.0.0.1", "success")
	}

	// 第 2 页（pageSize=2）
	logs, total, err := QueryAuditLogs(2, 2, "", "")
	if err != nil {
		t.Fatalf("QueryAuditLogs page 2 failed: %v", err)
	}
	if total != 5 {
		t.Errorf("总数应为 5，实际 %d", total)
	}
	if len(logs) != 2 {
		t.Errorf("第 2 页应返回 2 条，实际 %d", len(logs))
	}

	// 第 3 页（仅 1 条）
	logs, _, err = QueryAuditLogs(3, 2, "", "")
	if err != nil {
		t.Fatalf("QueryAuditLogs page 3 failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("第 3 页应返回 1 条，实际 %d", len(logs))
	}

	// 超出页码
	logs, _, err = QueryAuditLogs(10, 2, "", "")
	if err != nil {
		t.Fatalf("QueryAuditLogs overflow page failed: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("超出页码应返回 0 条，实际 %d", len(logs))
	}
}

// TestQueryAuditStats 测试统计接口。
func TestQueryAuditStats(t *testing.T) {
	setupTestDB(t)

	// 写入 3 条 search + 2 条 export
	for i := 0; i < 3; i++ {
		_ = AppendAuditLog("admin", "search", "/logs/search", nil, "127.0.0.1", "success")
	}
	for i := 0; i < 2; i++ {
		_ = AppendAuditLog("viewer", "export", "/logs/query", nil, "192.168.1.1", "success")
	}

	byAction, byDate, err := QueryAuditStats()
	if err != nil {
		t.Fatalf("QueryAuditStats failed: %v", err)
	}

	// 验证 byAction
	if len(byAction) != 2 {
		t.Errorf("期望 2 个 action 分组，实际 %d", len(byAction))
	}
	actionMap := map[string]int{}
	for _, a := range byAction {
		actionMap[a.Action] = a.Count
	}
	if actionMap["search"] != 3 {
		t.Errorf("期望 search 计数 3，实际 %d", actionMap["search"])
	}
	if actionMap["export"] != 2 {
		t.Errorf("期望 export 计数 2，实际 %d", actionMap["export"])
	}

	// 验证 byDate（至少 1 条今日数据）
	if len(byDate) == 0 {
		t.Error("期望至少 1 天数据")
	}
	totalByDate := 0
	for _, d := range byDate {
		totalByDate += d.Count
	}
	if totalByDate != 5 {
		t.Errorf("期望日期分组总数 5，实际 %d", totalByDate)
	}
}

// TestMain 包级数据库初始化的保护（实际由测试框架处理）。
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
