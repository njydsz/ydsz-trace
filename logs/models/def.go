package models

import (
	"database/sql"
	"fmt"
	"time"

	"ydsz-trace/pkg/config"

	_ "github.com/go-sql-driver/mysql"
)

// TClient 客户端
type TClient struct {
	Id          int64     `json:"id"`
	Ip          string    `json:"ip"`
	Port        string    `json:"port"`
	Vkey        string    `json:"vkey"`
	Info        string    `json:"info"`
	Zip         string    `json:"zip"`
	Online      string    `json:"online"`
	Status      string    `json:"status"`
	CreatedBy   string    `json:"createdBy"`
	CreatedTime time.Time `json:"createdTime"`
	UpdatedBy   string    `json:"updatedBy"`
	UpdatedTime time.Time `json:"updatedTime"`
}

// TItem 项目日志
type TItem struct {
	Id          int64     `json:"id"`
	ClientId    int64     `json:"clientId"`
	ItemName    string    `json:"itemName"`
	ItemDesc    string    `json:"itemDesc"`
	LogPath     string    `json:"logPath"`
	LogPrefix   string    `json:"logPrefix"`
	LogSuffix   string    `json:"logSuffix"`
	Status      string    `json:"status"`
	CreatedBy   string    `json:"createdBy"`
	CreatedTime time.Time `json:"createdTime"`
	UpdatedBy   string    `json:"updatedBy"`
	UpdatedTime time.Time `json:"updatedTime"`
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

// DBConfig 数据相关配置
type DBConfig struct {
	Host         string
	Port         string
	Database     string
	Username     string
	Password     string
	MaxIdleConns int //最大空闲连接
	MaxOpenConns int //最大连接数
}

// DB 全局数据库句柄
var DB *sql.DB

// InitDB 初始化数据库连接
func InitDB(cfg *config.Config) error {
	host := config.EnvOrConfig("YDSZ_DB_HOST", cfg.String("sqlhost"), "127.0.0.1")
	port := config.EnvOrConfig("YDSZ_DB_PORT", cfg.String("sqlport"), "3306")
	user := config.EnvOrConfig("YDSZ_DB_USER", cfg.String("sqluser"), "root")
	pwd := config.EnvOrConfig("YDSZ_DB_PASSWORD", cfg.String("sqlpwd"), "")
	database := config.EnvOrConfig("YDSZ_DB_NAME", cfg.String("database"), "ydsz_trace")
	maxIdle := cfg.Int("maxIdleConns", 10)
	maxOpen := cfg.Int("maxOpenConns", 50)

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		user, pwd, host, port, database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	db.SetMaxIdleConns(maxIdle)
	db.SetMaxOpenConns(maxOpen)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}

	DB = db
	return nil
}

// PageUtil 分页工具
func PageUtil(count int, pageNo int, pageSize int, list interface{}) Page {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
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
