// Package migration 子文件：v1 -> v2 迁移工具测试（FR-307）。
package migration

import (
	"context"
	"path/filepath"
	"testing"

	models "ydsz-trace/logs/models"
)

// setupTestDB 初始化临时测试数据库。
func setupTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_migration.db")
	cfg := &models.SQLiteConfig{FilePath: dbPath}
	if err := models.InitDB(cfg); err != nil {
		t.Fatalf("初始化测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if models.DB != nil {
			_ = models.DB.Close()
		}
	})
}

// TestRunAll_SchemaReady 迁移前 schema 就绪确认。
func TestRunAll_SchemaReady(t *testing.T) {
	setupTestDB(t)
	m := NewMigration()
	result, err := m.RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll 失败: %v", err)
	}
	if result["schema_ready"] != 1 {
		t.Errorf("schema_ready = %v want 1", result["schema_ready"])
	}
	if result["tables_ok"] != len(tablesToCheck) {
		t.Errorf("tables_ok = %v want %d", result["tables_ok"], len(tablesToCheck))
	}
	if result["backup_done"] != 1 {
		t.Errorf("backup_done = %v want 1", result["backup_done"])
	}
	backupPath, ok := result["backup_path"].(string)
	if !ok || backupPath == "" {
		t.Errorf("backup_path 为空或类型错误: %v", result["backup_path"])
	}
}

// TestRunAll_Idempotent 迁移幂等性：重复执行不报错。
func TestRunAll_Idempotent(t *testing.T) {
	setupTestDB(t)
	m := NewMigration()
	for i := 0; i < 3; i++ {
		_, err := m.RunAll(context.Background())
		if err != nil {
			t.Fatalf("第 %d 次 RunAll 失败: %v", i+1, err)
		}
	}
}

// TestStatus 迁移状态查询。
func TestStatus(t *testing.T) {
	setupTestDB(t)
	m := NewMigration()
	status, err := m.Status()
	if err != nil {
		t.Fatalf("Status 失败: %v", err)
	}
	counts, ok := status["tableCounts"].(map[string]int)
	if !ok {
		t.Fatalf("tableCounts 类型错误: %T", status["tableCounts"])
	}
	for _, table := range tablesToCheck {
		count, ok := counts[table]
		if !ok {
			t.Errorf("表 %s 不在 tableCounts 中", table)
			continue
		}
		if count < 0 {
			t.Errorf("表 %s 行数 = %d (查询失败)", table, count)
		}
	}
}

// TestListBackups 备份列表查询。
func TestListBackups(t *testing.T) {
	setupTestDB(t)
	m := NewMigration()

	// 先执行一次迁移（自动备份）。
	if _, err := m.RunAll(context.Background()); err != nil {
		t.Fatalf("RunAll 失败: %v", err)
	}

	backups, err := m.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups 失败: %v", err)
	}
	if len(backups) == 0 {
		t.Error("应有至少 1 份备份")
	}
}

// TestRestore_Rollback 一键回滚（FR-307）。
func TestRestore_Rollback(t *testing.T) {
	setupTestDB(t)
	m := NewMigration()

	// 插入一条 v1 数据（模拟迁移前状态）。
	_, err := models.DB.Exec(`
		INSERT INTO t_client (ip, port, vkey, status, online)
		VALUES ('127.0.0.1', '2020', 'vkey1', '1', '0');
	`)
	if err != nil {
		t.Fatalf("插入测试数据失败: %v", err)
	}

	// 执行迁移（自动备份）。
	result, err := m.RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll 失败: %v", err)
	}
	backupPath, _ := result["backup_path"].(string)
	backupFile := filepath.Base(backupPath)

	// 验证备份存在。
	backups, err := m.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups 失败: %v", err)
	}
	found := false
	for _, b := range backups {
		if b.FileName == backupFile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("备份文件 %s 不在列表中", backupFile)
	}

	// 删除数据（模拟迁移后状态变化）。
	if _, err := models.DB.Exec("DELETE FROM t_client"); err != nil {
		t.Fatalf("删除数据失败: %v", err)
	}
	var count int
	if err := models.DB.Get(&count, "SELECT COUNT(*) FROM t_client"); err != nil {
		t.Fatalf("查询行数失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("删除后行数 = %d want 0", count)
	}

	// 回滚。
	if err := m.Restore(context.Background(), backupFile); err != nil {
		t.Fatalf("Restore 失败: %v", err)
	}

	// 验证数据恢复。
	if err := models.DB.Get(&count, "SELECT COUNT(*) FROM t_client"); err != nil {
		t.Fatalf("回滚后查询行数失败: %v", err)
	}
	if count != 1 {
		t.Errorf("回滚后行数 = %d want 1", count)
	}
}

// TestRestore_EmptyBackupFile 空备份文件名应拒绝。
func TestRestore_EmptyBackupFile(t *testing.T) {
	setupTestDB(t)
	m := NewMigration()
	err := m.Restore(context.Background(), "")
	if err == nil {
		t.Error("空 backupFile 应返回错误")
	}
}

// TestRestore_InvalidBackupFile 非法备份文件名应拒绝。
func TestRestore_InvalidBackupFile(t *testing.T) {
	setupTestDB(t)
	m := NewMigration()
	err := m.Restore(context.Background(), "../../../etc/passwd")
	if err == nil {
		t.Error("非法 backupFile 应返回错误")
	}
}

// TestTryMigrateAddColumns 字段补全幂等。
func TestTryMigrateAddColumns(t *testing.T) {
	setupTestDB(t)
	for i := 0; i < 3; i++ {
		if err := models.TryMigrateAddColumns(); err != nil {
			t.Fatalf("第 %d 次 TryMigrateAddColumns 失败: %v", i+1, err)
		}
	}
}

// TestDBFilePath 数据库文件路径查询。
func TestDBFilePath(t *testing.T) {
	setupTestDB(t)
	path := models.DBFilePath()
	if path == "" {
		t.Error("DBFilePath 返回空")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("DBFilePath 不是绝对路径: %s", path)
	}
}
