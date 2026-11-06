package migration

import (
	"os"
	"path/filepath"
	"testing"
)

// TestImportFromV1_EmptyPath 空路径报错。
func TestImportFromV1_EmptyPath(t *testing.T) {
	_, err := ImportFromV1(nil, "", true)
	if err == nil {
		t.Fatal("空路径应返回错误")
	}
}

// TestImportFromV1_MissingFile 文件不存在报错。
func TestImportFromV1_MissingFile(t *testing.T) {
	_, err := ImportFromV1(nil, filepath.Join(t.TempDir(), "nope.db"), true)
	if err == nil {
		t.Fatal("文件不存在应返回错误")
	}
}

// TestImportFromV1_DirNotFile 目录路径报错。
func TestImportFromV1_DirNotFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("目录应存在: %v", err)
	}
	// 目录存在但 sqlite 无法作为库打开
	_, err := ImportFromV1(nil, dir, true)
	if err == nil {
		t.Error("目录作为数据库应返回错误")
	}
}

// TestV1ImportResult_Fields 结果结构字段完整。
func TestV1ImportResult_Fields(t *testing.T) {
	r := &V1ImportResult{
		SourcePath:      "/x/v1.db",
		ClientsImported: 3,
		ItemsImported:   5,
		RulesImported:   2,
		ClientsSkipped:  1,
		ItemsSkipped:    0,
		RulesSkipped:    4,
		DurationMs:      120,
	}

	if r.ClientsImported != 3 || r.ItemsImported != 5 || r.RulesImported != 2 {
		t.Error("字段赋值错误")
	}
	if r.ClientsSkipped != 1 || r.RulesSkipped != 4 {
		t.Error("跳过计数字段错误")
	}
	if r.DurationMs != 120 {
		t.Error("耗时字段错误")
	}
}

// TestImportBatchSize_Constant 批量常量合理。
func TestImportBatchSize_Constant(t *testing.T) {
	if ImportBatchSize <= 0 || ImportBatchSize > 5000 {
		t.Errorf("ImportBatchSize = %d", ImportBatchSize)
	}
}

// TestV1AttachAlias_Constant 别名不与保留字冲突。
func TestV1AttachAlias_Constant(t *testing.T) {
	if V1AttachAlias == "main" || V1AttachAlias == "temp" {
		t.Errorf("别名不能使用 SQLite 保留名: %s", V1AttachAlias)
	}
}
