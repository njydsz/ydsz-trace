// Package migration v1 -> v2 元数据迁移工具。
//
// 由于新项目 DDL（models/schema）已包含所有需要的表（t_user, t_item, t_client,
// t_task, t_alert_rule, t_alert_event, t_audit_log, t_pattern_stat），此工具仅做
// "schema 就绪确认 + 状态查询"，不做数据迁移或删除操作。
package migration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// tablesToCheck 需要确认存在且可访问的核心表。
var tablesToCheck = []string{
	"t_user",
	"t_item",
	"t_client",
	"t_task",
	"t_alert_rule",
	"t_alert_event",
	"t_audit_log",
	"t_pattern_stat",
}

// Migration 迁移任务执行器。
// 直接使用 models.DB（全局 *sqlx.DB），不持有自有连接。
type Migration struct{}

// ensureSchemaLocked 确保 schema 就绪（SQLITE_MASTER 表可访问 + 核心表均存在）。
// 幂等操作：新项目 DDL 已通过 InitDB 建表，此处仅为二次确认。
func ensureSchemaLocked() error {
	if models.DB == nil {
		return errors.New("models.DB 未初始化，请先调用 models.InitDB")
	}
	// 确认每个核心表可访问（SELECT 1 LIMIT 0 不发磁盘 IO，仅校验表存在）。
	for _, table := range tablesToCheck {
		var dummy int
		err := models.DB.Get(&dummy, fmt.Sprintf("SELECT 1 FROM %s LIMIT 0", table))
		if err != nil {
			return fmt.Errorf("表 %s 不可访问: %w", table, err)
		}
	}
	return nil
}

// NewMigration 创建迁移任务执行器。
func NewMigration() *Migration {
	return &Migration{}
}

// RunAll 执行全量迁移确认（幂等）。
// 新项目 DDL 已包含所有需要表，此接口确认 schema 就绪并返回状态。
func (m *Migration) RunAll(ctx context.Context) (map[string]int, error) {
	result := map[string]int{
		"schema_ready": 0,
		"tables_ok":    0,
	}

	if err := ensureSchemaLocked(); err != nil {
		log.Printf("[migration] schema 确认失败: %v", err)
		return result, fmt.Errorf("schema 确认失败: %w", err)
	}

	result["schema_ready"] = 1
	result["tables_ok"] = len(tablesToCheck)

	log.Printf("[migration] schema 就绪确认完成，共 %d 张核心表正常", len(tablesToCheck))
	return result, nil
}

// Status 查询迁移状态（哪些表存在且非空）。
func (m *Migration) Status() (map[string]bool, error) {
	status := map[string]bool{}

	if models.DB == nil {
		return status, errors.New("models.DB 未初始化")
	}

	for _, table := range tablesToCheck {
		var count int
		err := models.DB.Get(&count, fmt.Sprintf("SELECT COUNT(*) FROM %s", table))
		if err != nil {
			log.Printf("[migration] 查询 %s 状态失败: %v", table, err)
			status[table] = false
			continue
		}
		status[table] = count >= 0 // 表存在即可，count=0 也算正常（新项目空表）
	}

	return status, nil
}

// Rollback 空操作（提示不支持回滚），仅保留接口。
func (m *Migration) Rollback() error {
	return errors.New("当前不提供回滚：新项目 DDL 已包含所有表，无变更需要回滚")
}

// ========== HTTP Handler 层 ==========

// migrationInstance 全局单例（lazy init）。
var migrationInstance = NewMigration()

// Run 触发全量迁移确认。
// POST /admin/migration/run
func Run(c *gin.Context) {
	result, err := migrationInstance.RunAll(c.Request.Context())
	if err != nil {
		api.Fail(c, api.CodeServerError, fmt.Sprintf("迁移确认失败: %s", firstLine(err)))
		return
	}
	api.Success(c, "schema 就绪确认完成", result)
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

// RollbackHandler 空操作回滚（提示不支持）。
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
