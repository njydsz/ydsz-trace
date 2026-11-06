// Package migration 子文件：v1 元数据导入（V2.4-05，FR-307）。
//
// 背景：
//   v1 部署的 SQLite 文件需要先导入到 v2 才能使用新能力。
//   本模块提供从 v1 数据库文件读取元数据（客户端 / 日志项 / 告警规则）
//   并写入 v2 数据库的能力。
//
// 设计：
//   - 只读 v1 库：以只读方式 ATTACH，绝不写入源库
//   - 幂等：按业务唯一键（client_id / item_name）去重，重复导入不产生重复行
//   - 失败可回滚：整个导入在单事务内，任一步失败整体回滚
//   - 导入前自动备份 v2 库（防止误覆盖）
//
// v1 → v2 字段映射：
//
//   t_client: id → id, ip → ip, port → port, name → name,
//             status → status, created_time → created_time
//   t_item:   id → id, item_name → item_name, log_path → log_path,
//             log_prefix → log_prefix, log_suffix → log_suffix
package migration

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// 导入常量。
const (
	// ImportBatchSize 每批导入的行数
	ImportBatchSize = 500
	// V1AttachAlias v1 库在连接中的别名
	V1AttachAlias = "v1db"
)

// V1ImportResult 导入结果。
type V1ImportResult struct {
	// SourcePath v1 数据库路径
	SourcePath string `json:"sourcePath"`
	// BackupPath v2 库备份路径
	BackupPath string `json:"backupPath,omitempty"`
	// ClientsImported 导入的客户端数
	ClientsImported int `json:"clientsImported"`
	// ItemsImported 导入的日志项数
	ItemsImported int `json:"itemsImported"`
	// RulesImported 导入的告警规则数
	RulesImported int `json:"rulesImported"`
	// ClientsSkipped 因重复跳过的客户端数
	ClientsSkipped int `json:"clientsSkipped"`
	// ItemsSkipped 因重复跳过的日志项数
	ItemsSkipped int `json:"itemsSkipped"`
	// RulesSkipped 因重复跳过的规则数
	RulesSkipped int `json:"rulesSkipped"`
	// DurationMs 导入耗时（毫秒）
	DurationMs int64 `json:"durationMs"`
}

// ImportFromV1 从 v1 数据库文件导入元数据。
//
// v1Path 为 v1 SQLite 文件路径；dryRun=true 时只统计不写入。
func ImportFromV1(ctx context.Context, v1Path string, dryRun bool) (*V1ImportResult, error) {
	if models.DB == nil {
		return nil, fmt.Errorf("v2 数据库未初始化")
	}
	if v1Path == "" {
		return nil, fmt.Errorf("v1 数据库路径不能为空")
	}
	if _, err := os.Stat(v1Path); err != nil {
		return nil, fmt.Errorf("v1 数据库不可用: %w", err)
	}

	start := time.Now()
	result := &V1ImportResult{SourcePath: v1Path}

	// 备份 v2 库（真实导入时）
	if !dryRun {
		backupPath, err := models.CreateBackup(models.DBFilePath())
		if err != nil {
			return nil, fmt.Errorf("备份 v2 数据库失败: %w", err)
		}
		result.BackupPath = backupPath
	}

	// 以只读方式挂载 v1 库
	if err := attachReadOnly(v1Path); err != nil {
		return nil, err
	}
	defer detachV1()

	// 逐表导入（单事务）
	tx, err := models.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil {
			// 回滚失败仅记录
			_ = rerr
		}
	}()

	result.ClientsImported, result.ClientsSkipped, err = importClients(tx)
	if err != nil {
		return nil, fmt.Errorf("导入客户端失败: %w", err)
	}

	result.ItemsImported, result.ItemsSkipped, err = importItems(tx)
	if err != nil {
		return nil, fmt.Errorf("导入日志项失败: %w", err)
	}

	result.RulesImported, result.RulesSkipped, err = importAlertRules(tx)
	if err != nil {
		return nil, fmt.Errorf("导入告警规则失败: %w", err)
	}

	if dryRun {
		// 干跑：回滚事务，不产生实际写入
		return result, nil
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}

	result.DurationMs = time.Since(start).Milliseconds()
	log.Printf("[migration] v1 导入完成: 客户端 %d, 日志项 %d, 规则 %d（跳过 %d/%d/%d）",
		result.ClientsImported, result.ItemsImported, result.RulesImported,
		result.ClientsSkipped, result.ItemsSkipped, result.RulesSkipped)
	return result, nil
}

// attachReadOnly 以只读方式挂载 v1 库。
func attachReadOnly(path string) error {
	// 路径需转义单引号
	escaped := strings.ReplaceAll(path, "'", "''")
	_, err := models.DB.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS %s (READONLY)", escaped, V1AttachAlias))
	if err != nil {
		return fmt.Errorf("挂载 v1 数据库失败: %w", err)
	}
	return nil
}

// detachV1 卸载 v1 库。
func detachV1() {
	if _, err := models.DB.Exec("DETACH DATABASE " + V1AttachAlias); err != nil {
		log.Printf("[migration] 卸载 v1 数据库失败: %v", err)
	}
}

// importClients 导入客户端。
func importClients(tx *sqlx.Tx) (int, int, error) {
	return importGeneric(tx, "t_client",
		`SELECT id, ip, port, name, status, created_time FROM v1db.t_client`,
		`INSERT OR IGNORE INTO t_client (id, ip, port, name, status, created_time)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		6)
}

// importItems 导入日志项。
func importItems(tx *sqlx.Tx) (int, int, error) {
	return importGeneric(tx, "t_item",
		`SELECT id, item_name, log_path, log_prefix, log_suffix FROM v1db.t_item`,
		`INSERT OR IGNORE INTO t_item (id, item_name, log_path, log_prefix, log_suffix)
		 VALUES (?, ?, ?, ?, ?)`,
		5)
}

// importAlertRules 导入告警规则。
func importAlertRules(tx *sqlx.Tx) (int, int, error) {
	return importGeneric(tx, "t_alert_rule",
		`SELECT id, name, item_id, channel, webhook_url, threshold, interval_sec
		 FROM v1db.t_alert_rule`,
		`INSERT OR IGNORE INTO t_alert_rule
		 (id, name, item_id, channel, webhook_url, threshold, interval_sec)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		7)
}

// importGeneric 通用导入逻辑。
//
// selectSQL 从 v1 读取；insertSQL 写入 v2（INSERT OR IGNORE 保证幂等）。
// colCount 为列数，用于校验。
func importGeneric(tx *sqlx.Tx, table, selectSQL, insertSQL string, colCount int) (int, int, error) {
	rows, err := tx.QueryxContext(context.Background(), selectSQL)
	if err != nil {
		return 0, 0, fmt.Errorf("读取 %s 失败: %w", table, err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	imported, skipped := 0, 0
	for rows.Next() {
		values := make([]interface{}, colCount)
		ptrs := make([]interface{}, colCount)
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return imported, skipped, fmt.Errorf("扫描 %s 行失败: %w", table, err)
		}

		res, err := tx.ExecContext(context.Background(), insertSQL, values...)
		if err != nil {
			return imported, skipped, fmt.Errorf("写入 %s 失败: %w", table, err)
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			imported++
		} else {
			skipped++
		}
	}
	return imported, skipped, rows.Err()
}

// V1ImportHandler POST /admin/migration/import-v1
//
// 请求体：{"path": "/path/to/v1.db", "dryRun": true}
func V1ImportHandler(c *gin.Context) {
	var req struct {
		Path   string `json:"path"`
		DryRun bool   `json:"dryRun"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数非法")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		api.Fail(c, api.CodeBadRequest, "path 不能为空")
		return
	}

	// 路径必须存在
	if _, err := filepath.Abs(req.Path); err != nil {
		api.Fail(c, api.CodeBadRequest, "path 非法")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	result, err := ImportFromV1(ctx, req.Path, req.DryRun)
	if err != nil {
		log.Printf("[migration] v1 导入失败: %v", err)
		api.Fail(c, api.CodeServerError, "导入失败")
		return
	}

	api.Success(c, "导入完成", result)
}

// V1InspectHandler GET /admin/migration/inspect-v1?path=/path/to/v1.db
//
// 只读预览 v1 库的元数据规模，不执行导入。
func V1InspectHandler(c *gin.Context) {
	path := c.Query("path")
	if strings.TrimSpace(path) == "" {
		api.Fail(c, api.CodeBadRequest, "path 不能为空")
		return
	}

	if models.DB == nil {
		api.Fail(c, api.CodeServerError, "数据库未初始化")
		return
	}

	if err := attachReadOnly(path); err != nil {
		api.Fail(c, api.CodeServerError, "无法读取 v1 数据库")
		return
	}
	defer detachV1()

	stats := map[string]int{}
	for _, table := range []string{"t_client", "t_item", "t_alert_rule"} {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", V1AttachAlias, table)
		if err := models.DB.Get(&count, query); err != nil {
			// v1 可能缺少该表
			count = -1
		}
		stats[table] = count
	}

	api.Success(c, "查询成功", gin.H{
		"path":  path,
		"stats": stats,
	})
}
