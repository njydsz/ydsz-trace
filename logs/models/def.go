// Package models 包含 logs 服务端的 ORM 映射与数据库操作。
//
// 数据库使用 SQLite（文件存储，WAL 模式），表结构见 schema 常量。
package models

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// TClient 客户端实体（表 t_client）。
//
// source_type 字段区分客户端来源：
//   - file：传统 logc 实例（一台机器一个 logc）
//   - docker：Docker socket 模式的 logc 实例
//   - k8s：K8s DaemonSet 模式的 logc 实例
//
// identity 字段是虚拟客户端的稳定标识（容器 ID/UID+container），仅在 source_type 非 file 时有意义。
// 同一个 logc 可关联多个虚拟客户端 (virtual_parent 相同)。
type TClient struct {
	Id            int64  `json:"id" db:"id"`
	Ip            string `json:"ip" db:"ip"`
	Port          string `json:"port" db:"port"`
	Vkey          string `json:"vkey" db:"vkey"`
	Info          string `json:"info" db:"info"`
	Zip           string `json:"zip" db:"zip"`
	Status        string `json:"status" db:"status"`
	Online        string `json:"online" db:"online"`
	SourceType    string `json:"sourceType" db:"source_type"`
	Identity      string `json:"identity" db:"identity"`
	VirtualParent string `json:"virtualParent" db:"virtual_parent"`
	Labels        string `json:"labels" db:"labels"` // JSON 字符串，存储扩展标签
	CreatedBy     string `json:"createdBy" db:"created_by"`
	CreatedTime   string `json:"createdTime" db:"created_time"`
	UpdatedBy     string `json:"updatedBy" db:"updated_by"`
	UpdatedTime   string `json:"updatedTime" db:"updated_time"`
}

// TItem 项目日志实体（表 t_item）。
type TItem struct {
	Id          int64  `json:"id" db:"id"`
	ClientId    int64  `json:"clientId" db:"client_id"`
	ItemName    string `json:"itemName" db:"item_name"`
	ItemDesc    string `json:"itemDesc" db:"item_desc"`
	LogPath     string `json:"logPath" db:"log_path"`
	LogPrefix   string `json:"logPrefix" db:"log_prefix"`
	LogSuffix   string `json:"logSuffix" db:"log_suffix"`
	Status      string `json:"status" db:"status"`
	CreatedBy   string `json:"createdBy" db:"created_by"`
	CreatedTime string `json:"createdTime" db:"created_time"`
	UpdatedBy   string `json:"updatedBy" db:"updated_by"`
	UpdatedTime string `json:"updatedTime" db:"updated_time"`
}

// Page 通用分页响应结构体。
type Page struct {
	PageNo     int         `json:"pageNo"`
	PageSize   int         `json:"pageSize"`
	TotalPage  int         `json:"totalPage"`
	TotalCount int         `json:"totalCount"`
	FirstPage  bool        `json:"firstPage"`
	LastPage   bool        `json:"lastPage"`
	List       interface{} `json:"list"`
}

// SQLiteConfig SQLite 数据库文件路径配置。
type SQLiteConfig struct {
	FilePath string // 数据库文件路径，如 ./data/ydsz_trace.db
}

// DB 全局数据库句柄，通过 InitDB 初始化 SQLite 连接。
var DB *sqlx.DB

// schema 自动建表 SQL（幂等操作）。
// source_type/identity/virtual_parent/labels 字段由 TryMigrateV1 按需追加。
const schema = `
CREATE TABLE IF NOT EXISTS t_client (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    ip             TEXT,
    port           TEXT,
    vkey           TEXT,
    info           TEXT,
    zip            TEXT    DEFAULT '1',
    status         TEXT    DEFAULT '1',
    online         TEXT    DEFAULT '0',
    source_type    TEXT    DEFAULT 'file',
    identity       TEXT    DEFAULT '',
    virtual_parent TEXT    DEFAULT '',
    labels         TEXT    DEFAULT '{}',
    created_by     TEXT,
    created_time   TEXT    DEFAULT (datetime('now', 'localtime')),
    updated_by     TEXT,
    updated_time   TEXT    DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_t_client_ip_port ON t_client(ip, port);
CREATE INDEX IF NOT EXISTS idx_t_client_source ON t_client(source_type);
CREATE INDEX IF NOT EXISTS idx_t_client_identity ON t_client(identity);
CREATE INDEX IF NOT EXISTS idx_t_client_virtual_parent ON t_client(virtual_parent);

CREATE TABLE IF NOT EXISTS t_item (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id    INTEGER NOT NULL,
    item_name    TEXT,
    item_desc    TEXT,
    log_path     TEXT,
    log_prefix   TEXT,
    log_suffix   TEXT,
    status       TEXT    DEFAULT '1',
    created_by   TEXT,
    created_time TEXT    DEFAULT (datetime('now', 'localtime')),
    updated_by   TEXT,
    updated_time TEXT    DEFAULT (datetime('now', 'localtime')),
    FOREIGN KEY (client_id) REFERENCES t_client(id)
);
CREATE INDEX IF NOT EXISTS idx_t_item_client_id ON t_item(client_id);
`

// InitDB 初始化 SQLite 数据库连接。
//
// SQLite 使用文件作为存储，DSN 格式：file:path?_journal=WAL&_busy_timeout=5000
// WAL 模式提供更好的并发读写性能。
func InitDB(conf *SQLiteConfig) error {
	dsn := fmt.Sprintf("file:%s?_journal=WAL&_busy_timeout=5000&_synchronous=NORMAL", conf.FilePath)
	log.Printf("sqlite path: %s", conf.FilePath)

	db, err := sqlx.Connect("sqlite", dsn)
	if err != nil {
		return err
	}

	// SQLite 是单文件数据库，连接池设为 1 即可（避免写冲突）
	// 读多写少场景下可以适当调高，但必须开启 WAL
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return err
	}

	// 启用外键约束（SQLite 默认关闭）
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		log.Printf("enable foreign keys failed: %v", err)
	}

	// 自动建表（幂等）。IF NOT EXISTS 保证首次启动自动建表。
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("auto migrate schema failed: %w", err)
	}

	// 延申字段迁移（已有数据库不会自动新增列，需要 ALTER TABLE）
	if err := tryMigrateAddColumns(db); err != nil {
		log.Printf("check/add new columns: %v", err)
	}

	DB = db
	log.Printf("SQLite 数据库初始化完成: %s", conf.FilePath)
	return nil
}

// tryMigrateAddColumns 尝试为已有库补充新加的列（source_type / identity / virtual_parent / labels）。
// SQLite 不支持 ALTER TABLE IF NOT EXISTS；我们捕获 "duplicate column" 错误并忽略。
func tryMigrateAddColumns(db *sqlx.DB) error {
	columns := []struct {
		name    string
		sqlType string
		def     string
	}{
		{"source_type", "TEXT", "DEFAULT 'file'"},
		{"identity", "TEXT", "DEFAULT ''"},
		{"virtual_parent", "TEXT", "DEFAULT ''"},
		{"labels", "TEXT", "DEFAULT '{}'"},
	}
	for _, col := range columns {
		_, err := db.Exec(fmt.Sprintf(
			"ALTER TABLE t_client ADD COLUMN %s %s %s;",
			col.name, col.sqlType, col.def,
		))
		if err != nil && !contains(err.Error(), "duplicate column") {
			return fmt.Errorf("add column %s: %w", col.name, err)
		}
	}
	return nil
}

// contains 简易封装（strings.Contains 适配 bytes 风格字符串）。
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// PageUtil 构建分页响应结构（计算总页数、首/末页标志）。
func PageUtil(count int, pageNo int, pageSize int, list interface{}) Page {
	tp := count / pageSize
	if count%pageSize > 0 {
		tp = count/pageSize + 1
	}
	return Page{
		PageNo:     pageNo,
		PageSize:   pageSize,
		TotalPage:  tp,
		TotalCount: count,
		FirstPage:  pageNo == 1,
		LastPage:   pageNo == tp,
		List:       list,
	}
}
