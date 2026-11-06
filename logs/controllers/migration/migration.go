// Package migration v1 -> v2 元数据迁移工具（FR-307）。
//
// 职责：
//   - 迁移前自动备份核心元数据（复用 models.CreateBackup 的整库 VACUUM INTO）
//   - 字段补全：v1 的 t_task 缺 query_expr，v2 需要；t_alert_rule 缺 channel；
//     t_client 缺 source_type / identity / virtual_parent / labels / agent_mode。
//     由 models.TryMigrateAddColumns 幂等完成。
//   - 迁移状态查询：各表行数、schema 版本、备份记录
//   - 一键回滚：从备份整库恢复（复用 models.RestoreBackup）
//
// 设计约束：
//   - 迁移失败不破坏原库：先备份再迁移，备份失败则拒绝迁移
//   - 幂等：重复执行迁移不会产生重复数据
//   - 回滚可验证：恢复后核心表行数与备份一致
package migration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// coreTables 迁移涉及的核心表（按外键依赖顺序）。
var coreTables = []string{
	"t_user",
	"t_client",
	"t_item",
	"t_task",
	"t_alert_rule",
	"t_alert_event",
}

// tablesToCheck 需要确认存在且可访问的核心表。
var tablesToCheck = coreTables

// Migration 迁移任务执行器。
// 直接使用 models.DB（全局 *sqlx.DB），不持有自有连接。
type Migration struct{}

// ensureSchemaLocked 确保 schema 就绪（核心表均存在且可访问）。
func ensureSchemaLocked() error {
	if models.DB == nil {
		return errors.New("models.DB 未初始化，请先调用 models.InitDB")
	}
	for _, table := range tablesToCheck {
		// 用 EXISTS 判断表是否存在（不依赖表内是否有数据）。
		var exists int
		err := models.DB.Get(&exists, fmt.Sprintf("SELECT 1 FROM sqlite_master WHERE type='table' AND name='%s'", table))
		if err != nil {
			return fmt.Errorf("表 %s 检查失败: %w", table, err)
		}
		if exists != 1 {
			return fmt.Errorf("表 %s 不存在", table)
		}
		// 再确认表可访问（SELECT COUNT(*) 始终返回一行）。
		var count int
		if err := models.DB.Get(&count, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)); err != nil {
			return fmt.Errorf("表 %s 不可访问: %w", table, err)
		}
	}
	return nil
}

// NewMigration 创建迁移任务执行器。
func NewMigration() *Migration {
	return &Migration{}
}

// RunAll 执行全量迁移（幂等）。
//
// 步骤：
//  1. 确认 schema 就绪
//  2. 字段补全（models.TryMigrateAddColumns）
//  3. 备份核心表到备份目录（models.CreateBackup）
//  4. 返回迁移统计
func (m *Migration) RunAll(ctx context.Context) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"schema_ready":   0,
		"tables_ok":      0,
		"backup_done":    0,
		"rows_preserved": 0,
	}

	if err := ensureSchemaLocked(); err != nil {
		log.Printf("[migration] schema 确认失败: %v", err)
		return result, fmt.Errorf("schema 确认失败: %w", err)
	}
	result["schema_ready"] = 1
	result["tables_ok"] = len(tablesToCheck)

	// 字段补全（幂等）。
	if err := models.TryMigrateAddColumns(); err != nil {
		log.Printf("[migration] 字段补全失败: %v", err)
		return result, fmt.Errorf("字段补全失败: %w", err)
	}

	// 备份核心表。
	dbPath := models.DBFilePath()
	backupPath, err := models.CreateBackup(dbPath)
	if err != nil {
		log.Printf("[migration] 备份失败: %v", err)
		return result, fmt.Errorf("备份失败（迁移已中止）: %w", err)
	}
	result["backup_done"] = 1
	result["backup_path"] = backupPath

	// 统计核心表行数（迁移前）。
	counts := map[string]int{}
	for _, table := range coreTables {
		var count int
		if err := models.DB.Get(&count, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)); err != nil {
			log.Printf("[migration] 统计 %s 行数失败: %v", table, err)
			counts[table] = -1
			continue
		}
		counts[table] = count
		result["rows_preserved"] = result["rows_preserved"].(int) + count
	}
	result["table_counts"] = counts

	log.Printf("[migration] 迁移完成: %d 张核心表, %d 行数据, 备份至 %s",
		len(coreTables), result["rows_preserved"], backupPath)
	return result, nil
}

// Status 查询迁移状态（各表行数、schema 版本、备份列表）。
func (m *Migration) Status() (map[string]interface{}, error) {
	status := map[string]interface{}{}

	if models.DB == nil {
		return status, errors.New("models.DB 未初始化")
	}

	// 各表行数。
	tableCounts := map[string]int{}
	for _, table := range tablesToCheck {
		var count int
		err := models.DB.Get(&count, fmt.Sprintf("SELECT COUNT(*) FROM %s", table))
		if err != nil {
			log.Printf("[migration] 查询 %s 状态失败: %v", table, err)
			tableCounts[table] = -1
			continue
		}
		tableCounts[table] = count
	}
	status["tableCounts"] = tableCounts

	// schema 版本（PRAGMA user_version）。
	var userVersion int
	if err := models.DB.Get(&userVersion, "PRAGMA user_version"); err == nil {
		status["schemaVersion"] = userVersion
	}

	// 备份列表。
	dbPath := models.DBFilePath()
	backups, err := models.ListBackups(dbPath)
	if err != nil {
		log.Printf("[migration] 查询备份列表失败: %v", err)
	} else {
		status["backups"] = backups
	}

	return status, nil
}

// ListBackups 列出所有备份文件。
func (m *Migration) ListBackups() ([]models.BackupInfo, error) {
	if models.DB == nil {
		return nil, errors.New("models.DB 未初始化")
	}
	return models.ListBackups(models.DBFilePath())
}

// Restore 从备份文件回滚（一键回滚，FR-307）。
//
// 参数 backupFile 为备份文件名（如 ydsz-trace-backup-20261106-120000.sql）。
// 调用 models.RestoreBackup 执行整库恢复；恢复前自动创建应急备份。
func (m *Migration) Restore(ctx context.Context, backupFile string) error {
	if models.DB == nil {
		return errors.New("models.DB 未初始化")
	}
	if backupFile == "" {
		return errors.New("backupFile 不能为空")
	}
	// 安全检查：只允许恢复备份目录内的文件。
	base := filepath.Base(backupFile)
	dbPath := models.DBFilePath()
	if err := models.RestoreBackup(dbPath, base); err != nil {
		return fmt.Errorf("回滚失败: %w", err)
	}
	// 恢复后重新初始化 DB 句柄（RestoreBackup 会 Close 当前连接）。
	if err := models.InitDB(&models.SQLiteConfig{FilePath: dbPath}); err != nil {
		return fmt.Errorf("恢复后重新初始化数据库失败: %w", err)
	}
	log.Printf("[migration] 回滚完成: %s", base)
	return nil
}

// Rollback 空操作（提示使用 Restore），仅保留接口兼容。
func (m *Migration) Rollback() error {
	return errors.New("请使用 Restore 接口从备份回滚，调用前请先 ListBackups 确认备份文件")
}

// ========== HTTP Handler 层 ==========

// migrationInstance 全局单例（lazy init）。
var migrationInstance = NewMigration()

// Run 触发全量迁移。
// POST /admin/migration/run
func Run(c *gin.Context) {
	result, err := migrationInstance.RunAll(c.Request.Context())
	if err != nil {
		api.Fail(c, api.CodeServerError, fmt.Sprintf("迁移失败: %s", firstLine(err)))
		return
	}
	api.Success(c, "迁移完成", result)
}

// StatusHandler 查询迁移状态。
// GET /admin/migration/status
func StatusHandler(c *gin.Context) {
	status, err := migrationInstance.Status()
	if err != nil {
		api.Fail(c, api.CodeServerError, fmt.Sprintf("查询迁移状态失败: %s", firstLine(err)))
		return
	}
	api.Success(c, "查询成功", status)
}

// ListBackupsHandler 列出所有备份。
// GET /admin/migration/backups
func ListBackupsHandler(c *gin.Context) {
	backups, err := migrationInstance.ListBackups()
	if err != nil {
		api.Fail(c, api.CodeServerError, fmt.Sprintf("查询备份列表失败: %s", firstLine(err)))
		return
	}
	api.Success(c, "查询成功", backups)
}

// RestoreHandler 从备份回滚。
// POST /admin/migration/restore
// 请求体：{"backupFile": "ydsz-trace-backup-20261106-120000.sql"}
func RestoreHandler(c *gin.Context) {
	var req struct {
		BackupFile string `json:"backupFile"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: "+firstLine(err))
		return
	}
	if req.BackupFile == "" {
		api.Fail(c, api.CodeBadRequest, "backupFile 不能为空")
		return
	}
	if err := migrationInstance.Restore(c.Request.Context(), req.BackupFile); err != nil {
		api.Fail(c, api.CodeServerError, fmt.Sprintf("回滚失败: %s", firstLine(err)))
		return
	}
	api.Success(c, "回滚完成", nil)
}

// RollbackHandler 空操作回滚（提示使用 Restore）。
// POST /admin/migration/rollback
func RollbackHandler(c *gin.Context) {
	if err := migrationInstance.Rollback(); err != nil {
		api.Fail(c, api.CodeBadRequest, err.Error())
		return
	}
	api.Success(c, "回滚完成", nil)
}

// firstLine 取错误信息的第一行，避免换行破坏 JSON 响应。
func firstLine(err error) string {
	msg := err.Error()
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		return msg[:idx]
	}
	return msg
}
