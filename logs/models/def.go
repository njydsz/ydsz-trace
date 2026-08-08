package models

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// TClient 客户端
type TClient struct {
	Id          int64     `json:"id" db:"id"`
	Ip          string    `json:"ip" db:"ip"`
	Port        string    `json:"port" db:"port"`
	Vkey        string    `json:"vkey" db:"vkey"`
	Info        string    `json:"info" db:"info"`
	Zip         string    `json:"zip" db:"zip"`
	Online      string    `json:"online" db:"online"`
	Status      string    `json:"status" db:"status"`
	CreatedBy   string    `json:"createdBy" db:"created_by"`
	CreatedTime string    `json:"createdTime" db:"created_time"`
	UpdatedBy   string    `json:"updatedBy" db:"updated_by"`
	UpdatedTime string    `json:"updatedTime" db:"updated_time"`
}

// TItem 项目
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

// Page 分页
type Page struct {
	PageNo     int         `json:"pageNo"`
	PageSize   int         `json:"pageSize"`
	TotalPage  int         `json:"totalPage"`
	TotalCount int         `json:"totalCount"`
	FirstPage  bool        `json:"firstPage"`
	LastPage   bool        `json:"lastPage"`
	List       interface{} `json:"list"`
}

// SQLiteConfig SQLite 数据库配置
type SQLiteConfig struct {
	FilePath string // 数据库文件路径，如 ./data/ydsz_trace.db
}

// DB 全局数据库句柄
var DB *sqlx.DB

// InitDB 初始化 SQLite 数据库连接
//
// SQLite 使用文件作为存储，DSN 格式：file:path?_journal=WAL&_busy_timeout=5000
// WAL 模式提供更好的并发读写性能
func InitDB(conf *SQLiteConfig) error {
	dsn := fmt.Sprintf("file:%s?_journal=WAL&_busy_timeout=5000&_synchronous=NORMAL", conf.FilePath)
	log.Printf("sqlite dsn: %s", conf.FilePath)

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

	DB = db
	log.Printf("SQLite 数据库初始化完成: %s", conf.FilePath)
	return nil
}

// PageUtil 分页工具
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
