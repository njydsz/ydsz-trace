// Package models 子文件：备份与恢复（V2.1 FR-1307）。
//
// 提供 SQLite 数据库的逻辑备份（导出为 SQL dump）与恢复能力。
// 备份策略：
//   - 全量备份：导出所有表结构与数据
//   - 增量备份：基于 t_audit_log 的时间戳（可选）
//   - 自动清理：保留最近 N 份备份
package models

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	// backupDirName 备份文件存放目录名（相对于数据库文件所在目录）
	backupDirName = "backups"
	// backupPrefix 备份文件名前缀
	backupPrefix = "ydsz-trace-backup"
	// backupTimeFormat 备份文件名时间格式
	backupTimeFormat = "20060102-150405"
	// defaultBackupRetention 默认保留备份份数
	defaultBackupRetention = 7
)

// BackupInfo 备份文件信息。
type BackupInfo struct {
	FileName  string `json:"fileName"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
}

// CreateBackup 创建数据库逻辑备份（SQLite dump）。
//
// 使用 SQLite 的 `.dump` 命令导出全部表结构与数据。
// 备份文件命名：ydsz-trace-backup-YYYYMMDD-HHMMSS.sql
// 返回备份文件的完整路径。
func CreateBackup(dbPath string) (string, error) {
	if dbPath == "" {
		return "", fmt.Errorf("数据库路径为空")
	}

	// 确保备份目录存在
	backupDir := filepath.Join(filepath.Dir(dbPath), backupDirName)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("创建备份目录失败: %w", err)
	}

	// 构造备份文件路径
	timestamp := time.Now().Format(backupTimeFormat)
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s-%s.sql", backupPrefix, timestamp))

	// 使用 SQLite dump API（通过 sqlx 连接执行）
	// 注意：这里使用 VACUUM INTO 命令，它是 SQLite 3.27+ 支持的原生备份方式
	// 无需 shell 调用，线程安全且一致性好
	if DB == nil {
		return "", fmt.Errorf("数据库未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// VACUUM INTO 是 SQLite 原生的在线备份命令
	if _, err := DB.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", backupFile)); err != nil {
		return "", fmt.Errorf("VACUUM INTO 备份失败: %w", err)
	}

	log.Printf("[backup] 备份完成: %s", backupFile)
	return backupFile, nil
}

// ListBackups 列出指定数据库目录下的所有备份文件（按时间倒序）。
func ListBackups(dbPath string) ([]BackupInfo, error) {
	backupDir := filepath.Join(filepath.Dir(dbPath), backupDirName)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}

	backups := make([]BackupInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !isBackupFile(entry.Name()) {
			continue
		}
		backups = append(backups, BackupInfo{
			FileName:  entry.Name(),
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	// 按文件名倒序（即时间倒序）
	for i := 0; i < len(backups); i++ {
		for j := i + 1; j < len(backups); j++ {
			if backups[j].FileName > backups[i].FileName {
				backups[i], backups[j] = backups[j], backups[i]
			}
		}
	}

	return backups, nil
}

// RestoreBackup 从备份文件恢复数据库。
//
// 注意：恢复操作会覆盖当前数据库，调用方需确保已停止写入。
// 恢复前自动创建当前数据库的应急备份。
func RestoreBackup(dbPath, backupFile string) error {
	if dbPath == "" || backupFile == "" {
		return fmt.Errorf("数据库路径或备份文件为空")
	}

	// 安全检查：备份文件必须在备份目录内
	backupDir := filepath.Join(filepath.Dir(dbPath), backupDirName)
	fullBackupPath := filepath.Join(backupDir, filepath.Base(backupFile))
	if !isBackupFile(filepath.Base(backupFile)) {
		return fmt.Errorf("非法备份文件名: %s", backupFile)
	}

	// 应急备份：恢复前先备份当前数据库
	if _, err := CreateBackup(dbPath); err != nil {
		log.Printf("[restore] 应急备份失败（继续恢复）: %v", err)
	}

	// 关闭当前连接
	if DB != nil {
		if err := DB.Close(); err != nil {
			log.Printf("[restore] 关闭数据库连接失败: %v", err)
		}
	}

	// 删除当前数据库文件（保留 WAL 和 SHM）
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除当前数据库文件失败: %w", err)
	}
	// 清理 WAL/SHM
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	// 从备份恢复（VACUUM INTO 生成的是 SQLite 数据库文件，直接复制）
	if err := copyFile(fullBackupPath, dbPath); err != nil {
		return fmt.Errorf("复制备份文件失败: %w", err)
	}

	log.Printf("[restore] 数据库已从 %s 恢复", backupFile)
	return nil
}

// CleanupOldBackups 清理过期备份，保留最近 retention 份。
func CleanupOldBackups(dbPath string, retention int) (int, error) {
	if retention <= 0 {
		retention = defaultBackupRetention
	}
	backups, err := ListBackups(dbPath)
	if err != nil {
		return 0, err
	}
	if len(backups) <= retention {
		return 0, nil
	}

	backupDir := filepath.Join(filepath.Dir(dbPath), backupDirName)
	removed := 0
	for _, b := range backups[retention:] {
		fullPath := filepath.Join(backupDir, b.FileName)
		if err := os.Remove(fullPath); err != nil {
			log.Printf("[backup] 清理备份失败 %s: %v", b.FileName, err)
			continue
		}
		removed++
	}
	log.Printf("[backup] 已清理 %d 份过期备份", removed)
	return removed, nil
}

// isBackupFile 判断文件名是否为备份文件。
func isBackupFile(name string) bool {
	if len(name) < len(backupPrefix) {
		return false
	}
	return name[:len(backupPrefix)] == backupPrefix
}

// copyFile 复制文件（用于恢复）。
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := source.Close(); cerr != nil {
			log.Printf("[restore] 关闭源文件失败: %v", cerr)
		}
	}()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := destination.Close(); cerr != nil {
			log.Printf("[restore] 关闭目标文件失败: %v", cerr)
		}
	}()

	buf := make([]byte, 32*1024)
	for {
		n, err := source.Read(buf)
		if n > 0 {
			if _, werr := destination.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err == sql.ErrNoRows || err != nil {
			break
		}
	}
	return nil
}

// GetBackupDir 返回备份目录路径。
func GetBackupDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), backupDirName)
}
