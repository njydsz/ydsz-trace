// Package models 内部测试：验证 sqlite.sql 与 schema 常量的一致性。
package models

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSQLiteSQLTableNamesMatchSchema 验证 sqlite.sql 中包含的所有表名与 schema 常量一致。
func TestSQLiteSQLTableNamesMatchSchema(t *testing.T) {
	expectedTables := extractTableNamesFromSchema(t)
	sqlContent := readSQLiteSQL(t)

	for _, table := range expectedTables {
		// 检查 sqlite.sql 中是否包含对应 CREATE TABLE
		pattern := "CREATE TABLE " + table
		if !strings.Contains(sqlContent, pattern) {
			t.Errorf("sqlite.sql 缺少表 %q 的 CREATE TABLE 语句（期望 %q）", table, pattern)
		}
	}
}

// TestSQLiteSQLClientColumnsMatchSchema 验证 sqlite.sql 中 t_client 的列数不少于 schema 中的列数。
func TestSQLiteSQLClientColumnsMatchSchema(t *testing.T) {
	schemaColumns := extractTableColumns(schema, "t_client")
	sqlContent := readSQLiteSQL(t)
	sqlColumns := extractTableColumns(sqlContent, "t_client")

	if len(sqlColumns) < len(schemaColumns) {
		t.Errorf("sqlite.sql 中 t_client 列数 (%d) 少于 schema 常量 (%d)", len(sqlColumns), len(schemaColumns))
		t.Errorf("schema 期望列: %v", schemaColumns)
	}
}

// TestSQLiteSQLHasTaskTables 验证 sqlite.sql 包含 t_task / t_alert_rule / t_alert_event 表。
func TestSQLiteSQLHasTaskTables(t *testing.T) {
	sqlContent := readSQLiteSQL(t)

	requiredTables := []string{"t_task", "t_alert_rule", "t_alert_event"}
	for _, table := range requiredTables {
		if !strings.Contains(sqlContent, "CREATE TABLE "+table+" (") {
			t.Errorf("sqlite.sql 缺少 %s 表定义", table)
		}
	}
}

// extractTableNamesFromSchema 从 schema 常量中提取所有表名。
func extractTableNamesFromSchema(t *testing.T) []string {
	t.Helper()
	return extractTableColumnsNamesFromSQL(schema)
}

// extractTableColumnsNamesFromSQL 从 SQL 文本中提取所有 CREATE TABLE 的表名。
func extractTableColumnsNamesFromSQL(sql string) []string {
	var tables []string
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CREATE TABLE ") {
			// 跳过 IF NOT EXISTS
			trimmed = strings.TrimPrefix(trimmed, "CREATE TABLE ")
			trimmed = strings.TrimPrefix(trimmed, "IF NOT EXISTS ")
			// 提取表名（第一个空格或左括号前）
			if idx := strings.IndexAny(trimmed, " ("); idx > 0 {
				tables = append(tables, strings.TrimSpace(trimmed[:idx]))
			}
		}
	}
	return tables
}

// extractTableColumns 从 SQL 文本中提取指定表的列名列表。
func extractTableColumns(sql, tableName string) []string {
	var columns []string
	inTargetTable := false
	depth := 0

	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)

		// 检测 CREATE TABLE 开头
		if strings.HasPrefix(trimmed, "CREATE TABLE ") || strings.HasPrefix(trimmed, "CREATE TABLE IF NOT EXISTS ") {
			prefix := "CREATE TABLE "
			rest := strings.TrimPrefix(trimmed, prefix)
			rest = strings.TrimPrefix(rest, "IF NOT EXISTS ")
			if idx := strings.IndexAny(rest, " ("); idx > 0 {
				name := strings.TrimSpace(rest[:idx])
				if name == tableName {
					inTargetTable = true
					depth = 0
					// 继续处理当前行，统计括号
					for _, ch := range trimmed {
						switch ch {
						case '(':
							depth++
						case ')':
							depth--
						}
					}
					// 如果同一行就关闭了（如空表），退出
					if depth <= 0 {
						inTargetTable = false
					}
					continue
				}
			}
		}

		if !inTargetTable {
			continue
		}

		// 统计括号深度
		for _, ch := range trimmed {
			switch ch {
			case '(':
				depth++
			case ')':
				depth--
			}
		}

		// 遇到关闭括号，表定义结束
		if depth <= 0 && strings.Contains(trimmed, ")") {
			inTargetTable = false
			continue
		}

		// 跳过 FOREIGN KEY / PRIMARY KEY / UNIQUE / CHECK / CREATE INDEX 等行
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "FOREIGN") ||
			strings.HasPrefix(upper, "PRIMARY") ||
			strings.HasPrefix(upper, "UNIQUE") ||
			strings.HasPrefix(upper, "CHECK") ||
			strings.HasPrefix(upper, "CREATE") ||
			trimmed == "" {
			continue
		}

		// 提取列名（去掉末尾逗号）
		trimmed = strings.TrimSuffix(trimmed, ",")
		if idx := strings.IndexAny(trimmed, " \t"); idx > 0 {
			colName := strings.TrimSpace(trimmed[:idx])
			if colName != "" {
				columns = append(columns, colName)
			}
		}
	}

	return columns
}

// readSQLiteSQL 读取 sqls/sqlite.sql 文件内容。
// 运行时从当前文件位置向上查找 sqls/sqlite.sql。
func readSQLiteSQL(t *testing.T) string {
	t.Helper()

	// 从当前测试文件的目录出发，向上查找 sqls/sqlite.sql
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法获取当前测试文件路径")
	}
	baseDir := filepath.Dir(filename)

	// 尝试多种相对路径
	candidates := []string{
		filepath.Join(baseDir, "..", "..", "sqls", "sqlite.sql"),
		filepath.Join(baseDir, "..", "..", "..", "sqls", "sqlite.sql"),
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data)
		}
	}

	// 最后的尝试：从工作目录向上查找
	wd, _ := os.Getwd()
	for i := 0; i < 5; i++ {
		path := filepath.Join(wd, "sqls", "sqlite.sql")
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data)
		}
		wd = filepath.Dir(wd)
	}

	t.Fatal("无法找到 sqls/sqlite.sql 文件")
	return ""
}
