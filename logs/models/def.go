package models

import (
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/go-sql-driver/mysql"
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
	CreatedTime time.Time `json:"createdTime" db:"created_time"`
	UpdatedBy   string    `json:"updatedBy" db:"updated_by"`
	UpdatedTime time.Time `json:"updatedTime" db:"updated_time"`
}

// TItem 项目
type TItem struct {
	Id          int64     `json:"id" db:"id"`
	ClientId    int64     `json:"clientId" db:"client_id"`
	ItemName    string    `json:"itemName" db:"item_name"`
	ItemDesc    string    `json:"itemDesc" db:"item_desc"`
	LogPath     string    `json:"logPath" db:"log_path"`
	LogPrefix   string    `json:"logPrefix" db:"log_prefix"`
	LogSuffix   string    `json:"logSuffix" db:"log_suffix"`
	Status      string    `json:"status" db:"status"`
	CreatedBy   string    `json:"createdBy" db:"created_by"`
	CreatedTime time.Time `json:"createdTime" db:"created_time"`
	UpdatedBy   string    `json:"updatedBy" db:"updated_by"`
	UpdatedTime time.Time `json:"updatedTime" db:"updated_time"`
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
var DB *sqlx.DB

// InitDB 初始化数据库连接
func InitDB(conf *DBConfig) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=true&loc=Local",
		conf.Username, conf.Password, conf.Host, conf.Port, conf.Database)
	log.Printf("datasource: %s@tcp(%s:%s)/%s", conf.Username, conf.Host, conf.Port, conf.Database)

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return err
	}
	if conf.MaxIdleConns > 0 {
		db.SetMaxIdleConns(conf.MaxIdleConns)
	}
	if conf.MaxOpenConns > 0 {
		db.SetMaxOpenConns(conf.MaxOpenConns)
	}
	if err := db.Ping(); err != nil {
		return err
	}
	DB = db
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
