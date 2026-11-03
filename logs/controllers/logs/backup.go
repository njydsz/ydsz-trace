// Package logs 子文件：备份与恢复 API（V2.1 FR-1307）。
//
// 提供备份创建、列表查询、恢复、清理四个端点。
// 仅 admin 角色可访问。
package logs

import (
	"net/http"
	"os"
	"path/filepath"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/config"

	"github.com/gin-gonic/gin"
)

// BackupCreateHandler 创建数据库备份。
//
// @Summary 创建备份
// @Description 对 SQLite 数据库文件执行备份，返回备份文件路径。仅 admin 角色可访问。
// @Tags Backup
// @Produce json
// @Success 200 {object} api.Response{data=map[string]string} "path = 备份文件路径"
// @Failure 500 {object} api.Response "备份失败"
// @Router /logs/backup/create [post]
func BackupCreateHandler(c *gin.Context) {
	dbPath := resolveDBPath()
	backupPath, err := models.CreateBackup(dbPath)
	if err != nil {
		api.Fail(c, api.CodeServerError, "备份失败: "+err.Error())
		return
	}

	// 自动清理过期备份
	_, _ = models.CleanupOldBackups(dbPath, 7)

	api.Success(c, "备份成功", gin.H{"path": backupPath})
}

// BackupListHandler 列出所有备份文件。
//
// @Summary 备份列表
// @Description 列出所有可用的数据库备份文件及其元信息。仅 admin 角色可访问。
// @Tags Backup
// @Produce json
// @Success 200 {object} api.Response{data=[]map[string]interface{}}
// @Failure 500 {object} api.Response "查询失败"
// @Router /logs/backup/list [get]
func BackupListHandler(c *gin.Context) {
	dbPath := resolveDBPath()
	backups, err := models.ListBackups(dbPath)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询备份列表失败: "+err.Error())
		return
	}
	api.Success(c, "查询成功", backups)
}

// BackupRestoreHandler 从指定备份恢复数据库。
//
// @Summary 恢复备份
// @Description 从指定备份文件恢复 SQLite 数据库。恢复后服务需重启以加载新数据库。仅 admin 角色可访问。
// @Tags Backup
// @Accept json
// @Produce json
// @Param body body object{fileName=string} true "fileName: 备份文件名（必填）"
// @Success 200 {object} api.Response "恢复成功"
// @Failure 400 {object} api.Response "参数错误"
// @Failure 500 {object} api.Response "恢复失败"
// @Router /logs/backup/restore [post]
func BackupRestoreHandler(c *gin.Context) {
	var req struct {
		FileName string `json:"fileName" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: fileName 为必填项")
		return
	}

	dbPath := resolveDBPath()
	if err := models.RestoreBackup(dbPath, req.FileName); err != nil {
		api.Fail(c, api.CodeServerError, "恢复失败: "+err.Error())
		return
	}

	api.Success(c, "恢复成功，服务需要重启以加载新数据库", nil)
}

// BackupCleanupHandler 清理过期备份。
//
// @Summary 清理备份
// @Description 清理超过保留天数的过期备份文件。仅 admin 角色可访问。
// @Tags Backup
// @Produce json
// @Param retention query int false "保留天数（默认 7）"
// @Success 200 {object} api.Response{data=map[string]int} "removed = 清理数量"
// @Failure 500 {object} api.Response "清理失败"
// @Router /logs/backup/cleanup [post]
func BackupCleanupHandler(c *gin.Context) {
	retention := 7
	if s := c.Query("retention"); s != "" {
		if n := parseIntDefault(s, 7); n > 0 {
			retention = n
		}
	}

	dbPath := resolveDBPath()
	removed, err := models.CleanupOldBackups(dbPath, retention)
	if err != nil {
		api.Fail(c, api.CodeServerError, "清理失败: "+err.Error())
		return
	}

	api.Success(c, "清理完成", gin.H{"removed": removed})
}

// resolveDBPath 解析数据库文件路径。
func resolveDBPath() string {
	if v := os.Getenv("YDSZ_SQLITE_PATH"); v != "" {
		return v
	}
	// 默认值需与 main.go 中的 sqlitePath 保持一致
	cfgPath := config.EnvOrConfig("config.file", "", "")
	if cfgPath != "" {
		abs, err := filepath.Abs(cfgPath)
		if err == nil {
			baseDir := filepath.Dir(abs)
			return filepath.Join(baseDir, "ydsz-trace.db")
		}
	}
	return "ydsz-trace.db"
}

// 确保 net/http 被使用。
var _ = http.StatusOK
