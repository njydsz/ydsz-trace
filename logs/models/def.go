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

// TLogSegment 列式段存储结构（表 t_log_segment）。
type TLogSegment struct {
	Id          int64  `json:"id" db:"id"`
	ItemId      int64  `json:"itemId" db:"item_id"`
	ClientId    int64  `json:"clientId" db:"client_id"`
	SegDate     string `json:"segDate" db:"seg_date"`
	SegHour     string `json:"segHour" db:"seg_hour"`
	TemplateId  string `json:"templateId" db:"template_id"`
	LineCount   int    `json:"lineCount" db:"line_count"`
	ErrorCount  int    `json:"errorCount" db:"error_count"`
	WarnCount   int    `json:"warnCount" db:"warn_count"`
	FirstTs     string `json:"firstTs" db:"first_ts"`
	LastTs      string `json:"lastTs" db:"last_ts"`
	Compressed  []byte `json:"-" db:"compressed"` // zstd/gzip 压缩的列式数据，不参与 JSON 序列化
	CreatedTime string `json:"createdTime" db:"created_time"`
}

// TLogTemplate 日志模板（表 t_log_template）。
type TLogTemplate struct {
	Id           int64  `json:"id" db:"id"`
	TemplateId   string `json:"templateId" db:"template_id"`
	TemplateText string `json:"templateText" db:"template_text"`
	Service      string `json:"service" db:"service"`
	Level        string `json:"level" db:"level"`
	FirstSeen    string `json:"firstSeen" db:"first_seen"`
	LastSeen     string `json:"lastSeen" db:"last_seen"`
	TotalCount   int    `json:"totalCount" db:"total_count"`
	SampleLine   string `json:"sampleLine" db:"sample_line"`
	CreatedTime  string `json:"createdTime" db:"created_time"`
	UpdatedTime  string `json:"updatedTime" db:"updated_time"`
}

// TEmbedding embedding 向量索引元数据（表 t_embedding）。
type TEmbedding struct {
	Id           int64  `json:"id" db:"id"`
	TargetType   string `json:"targetType" db:"target_type"`
	TargetId     string `json:"targetId" db:"target_id"`
	ModelVersion string `json:"modelVersion" db:"model_version"`
	VectorDim    int    `json:"vectorDim" db:"vector_dim"`
	CreatedTime  string `json:"createdTime" db:"created_time"`
}

// TAnomalyEvent 异常事件（表 t_anomaly_event）。
type TAnomalyEvent struct {
	Id           int64   `json:"id" db:"id"`
	TemplateId   string  `json:"templateId" db:"template_id"`
	ItemId       int64   `json:"itemId" db:"item_id"`
	ClientId     int64   `json:"clientId" db:"client_id"`
	AnomalyType  string  `json:"anomalyType" db:"anomaly_type"`
	Score        float64 `json:"score" db:"score"`
	Baseline     float64 `json:"baseline" db:"baseline"`
	Actual       float64 `json:"actual" db:"actual"`
	WindowSec    int     `json:"windowSec" db:"window_sec"`
	Status       string  `json:"status" db:"status"`
	Description  string  `json:"description" db:"description"`
	CreatedTime  string  `json:"createdTime" db:"created_time"`
}

// TTraceNode 链路追踪节点（表 t_trace_node）。
type TTraceNode struct {
	Id          int64  `json:"id" db:"id"`
	TraceId     string `json:"traceId" db:"trace_id"`
	SpanId      string `json:"spanId" db:"span_id"`
	ParentId    string `json:"parentId" db:"parent_id"`
	Service     string `json:"service" db:"service"`
	Operation   string `json:"operation" db:"operation"`
	StartTs     string `json:"startTs" db:"start_ts"`
	DurationMs  int    `json:"durationMs" db:"duration_ms"`
	Status      string `json:"status" db:"status"`
	Tags        string `json:"tags" db:"tags"`
	CreatedTime string `json:"createdTime" db:"created_time"`
}

// TCaseLibrary 案例库（表 t_case_library）。
type TCaseLibrary struct {
	Id          int64  `json:"id" db:"id"`
	CaseId      string `json:"caseId" db:"case_id"`
	Title       string `json:"title" db:"title"`
	Symptoms    string `json:"symptoms" db:"symptoms"`
	RootCause   string `json:"rootCause" db:"root_cause"`
	Solution    string `json:"solution" db:"solution"`
	TagList     string `json:"tagList" db:"tag_list"`
	Source      string `json:"source" db:"source"`
	ConvId      string `json:"convId" db:"conv_id"`
	HitCount    int    `json:"hitCount" db:"hit_count"`
	CreatedBy   string `json:"createdBy" db:"created_by"`
	CreatedTime string `json:"createdTime" db:"created_time"`
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

CREATE TABLE IF NOT EXISTS t_task (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    task_no       TEXT    NOT NULL UNIQUE,
    user_name     TEXT    DEFAULT '',
    client_id     INTEGER NOT NULL,
    item_id       INTEGER NOT NULL,
    item_name     TEXT    NOT NULL,
    log_date      TEXT    NOT NULL,
    key_word      TEXT    DEFAULT '',
    regex         INTEGER DEFAULT 0,
    level         TEXT    DEFAULT '',
    start_time    TEXT    DEFAULT '',
    end_time      TEXT    DEFAULT '',
    query_expr    TEXT    DEFAULT '',
    line_count    INTEGER DEFAULT 0,
    max_lines     INTEGER DEFAULT 0,
    status        TEXT    DEFAULT 'pending',
    node_total    INTEGER DEFAULT 0,
    node_done     INTEGER DEFAULT 0,
    match_count   INTEGER DEFAULT 0,
    error_msg     TEXT    DEFAULT '',
    zip_path      TEXT    DEFAULT '',
    created_time  TEXT    DEFAULT (datetime('now', 'localtime')),
    started_time  TEXT    DEFAULT '',
    finished_time TEXT    DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_t_task_status ON t_task(status);
CREATE INDEX IF NOT EXISTS idx_t_task_created ON t_task(created_time);

-- 告警规则：按 item + 关键词/正则匹配的阈值告警，命中即 webhook 推送
CREATE TABLE IF NOT EXISTS t_alert_rule (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT    NOT NULL,
    item_id        INTEGER NOT NULL,
    client_id      INTEGER NOT NULL DEFAULT 0,         -- 0 表示所有客户端
    key_word       TEXT    DEFAULT '',
    regex          INTEGER DEFAULT 0,                  -- 0 = 包含匹配，1 = 正则
    level          TEXT    DEFAULT '',
    threshold      INTEGER DEFAULT 1,                  -- 命中 >= threshold 才触发
    interval_sec   INTEGER DEFAULT 300,                -- 评估间隔（秒），同时用作周期去重窗口
    webhook_url    TEXT    NOT NULL,                   -- webhook URL；email 渠道复用为 smtp:// 连接串
    channel        TEXT    DEFAULT 'webhook',          -- 通知渠道：webhook/wecom/dingtalk/feishu/email
    enabled        INTEGER DEFAULT 1,
    last_fired     TEXT    DEFAULT '',
    created_by     TEXT    DEFAULT '',
    created_time   TEXT    DEFAULT (datetime('now', 'localtime')),
    updated_time   TEXT    DEFAULT (datetime('now', 'localtime')),
    FOREIGN KEY (item_id) REFERENCES t_item(id)
);
CREATE INDEX IF NOT EXISTS idx_alert_rule_item ON t_alert_rule(item_id);
CREATE INDEX IF NOT EXISTS idx_alert_rule_enabled ON t_alert_rule(enabled);

-- 用户表：支持多账号登录，密码使用 argon2id 哈希存储
CREATE TABLE IF NOT EXISTS t_user (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    role          TEXT    DEFAULT 'viewer',
    status        INTEGER DEFAULT 1,
    created_time  TEXT    DEFAULT (datetime('now', 'localtime')),
    updated_time  TEXT    DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_t_user_username ON t_user(username);

-- 告警投递记录：每次触发 webhook 写一条，便于追溯与排错
CREATE TABLE IF NOT EXISTS t_alert_event (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id      INTEGER NOT NULL,
    rule_name    TEXT    DEFAULT '',
    webhook_url  TEXT    DEFAULT '',
    status       TEXT    DEFAULT 'ok',                -- ok / fail
    http_code    INTEGER DEFAULT 0,
    match_count  INTEGER DEFAULT 0,
    sample_text  TEXT    DEFAULT '',
    error_msg    TEXT    DEFAULT '',
    fired_time   TEXT    DEFAULT (datetime('now', 'localtime')),
    FOREIGN KEY (rule_id) REFERENCES t_alert_rule(id)
);
CREATE INDEX IF NOT EXISTS idx_alert_event_rule ON t_alert_event(rule_id);
CREATE INDEX IF NOT EXISTS idx_alert_event_time ON t_alert_event(fired_time);

-- 模式时序计数表：按 5min 桶聚合模板命中数，用于异常检测基线
CREATE TABLE IF NOT EXISTS t_pattern_stat (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id      INTEGER NOT NULL,
    client_id    INTEGER NOT NULL,
    template_hash TEXT   NOT NULL,
    time_bucket  TEXT    NOT NULL,
    count        INTEGER DEFAULT 0,
    first_time   TEXT,
    last_time    TEXT,
    sample       TEXT,
    created_time TEXT    DEFAULT (datetime('now','localtime')),
    UNIQUE(item_id, client_id, template_hash, time_bucket)
);
CREATE INDEX IF NOT EXISTS idx_pattern_stat_lookup ON t_pattern_stat(item_id, client_id, time_bucket);
CREATE INDEX IF NOT EXISTS idx_pattern_stat_template ON t_pattern_stat(template_hash);

-- 模型能力配置：管理 AI 模型（LLM/Embedding）的接入参数
CREATE TABLE IF NOT EXISTS t_model_capability (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT    NOT NULL UNIQUE,          -- 模型配置名称（如 GPT-4、本地 Embedding）
    provider      TEXT    NOT NULL,                 -- 提供商：openai / anthropic / ollama / custom
    model_id      TEXT    NOT NULL,                 -- 模型标识（如 gpt-4o、text-embedding-ada-002）
    api_key       TEXT    DEFAULT '',               -- API 密钥（加密存储，回显时脱敏）
    base_url      TEXT    DEFAULT '',               -- API 端点（空则使用提供商默认）
    temperature   REAL    DEFAULT 0.7,              -- 采样温度
    max_tokens    INTEGER DEFAULT 4096,             -- 最大输出 token 数
    enabled       INTEGER DEFAULT 1,                -- 是否启用：1-启用 0-禁用
    capability    TEXT    NOT NULL DEFAULT 'chat',  -- 能力类型：chat / embedding / rerank
    created_by    TEXT    DEFAULT '',
    created_time  TEXT    DEFAULT (datetime('now', 'localtime')),
    updated_time  TEXT    DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_model_capability_provider ON t_model_capability(provider);
CREATE INDEX IF NOT EXISTS idx_model_capability_enabled ON t_model_capability(enabled);

-- 审计日志：记录查询、导出、配置变更全量留痕
CREATE TABLE IF NOT EXISTS t_audit_log (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    username     TEXT,                       -- 操作用户
    action       TEXT    NOT NULL,           -- 操作类型：search/export/login/config_change/delete
    resource     TEXT,                       -- 操作对象：/logs/search, /client/register 等
    detail       TEXT,                       -- JSON 格式详情：搜索条件、影响行数等
    client_ip    TEXT,                       -- 请求方 IP
    result       TEXT    DEFAULT 'success',  -- success/fail
    created_time TEXT    DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_audit_username ON t_audit_log(username);
CREATE INDEX IF NOT EXISTS idx_audit_action ON t_audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_time ON t_audit_log(created_time);

-- V2.2 新增表：异常反馈与阈值自适应（FR-807）
CREATE TABLE IF NOT EXISTS t_anomaly_feedback (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    anomaly_id     INTEGER NOT NULL,        -- 关联 t_anomaly_event.id
    template_id    TEXT    DEFAULT '',      -- 冗余模板 hash，便于按模板聚合
    item_id        INTEGER DEFAULT 0,
    feedback_type  TEXT    NOT NULL,        -- false_positive / true_positive / ignored
    reason         TEXT    DEFAULT '',      -- 用户填写的原因
    adjusted_score REAL    DEFAULT 0,       -- 该模板自适应后的 score 阈值
    created_by     TEXT    DEFAULT '',
    created_time   TEXT    DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_anomaly_fb_template ON t_anomaly_feedback(template_id);
CREATE INDEX IF NOT EXISTS idx_anomaly_fb_type     ON t_anomaly_feedback(feedback_type);
CREATE INDEX IF NOT EXISTS idx_anomaly_fb_time     ON t_anomaly_feedback(created_time);

-- V2.2 新增表：告警静默窗口（维护期配置，FR-806）
CREATE TABLE IF NOT EXISTS t_maintenance_window (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    start_time TEXT    NOT NULL,            -- HH:MM
    end_time   TEXT    NOT NULL,            -- HH:MM（支持跨天）
    recurring  TEXT    DEFAULT 'daily',     -- daily / weekly / once
    weekdays   TEXT    DEFAULT '',          -- JSON 数组，weekly 时生效
    enabled    INTEGER DEFAULT 1,
    created_time TEXT  DEFAULT (datetime('now','localtime')),
    updated_time TEXT  DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_maint_window_enabled ON t_maintenance_window(enabled);

-- V2.1 新增表：列式段（Segment）存储结构
CREATE TABLE IF NOT EXISTS t_log_segment (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id      INTEGER NOT NULL,
    client_id    INTEGER NOT NULL,
    seg_date     TEXT    NOT NULL,          -- 日期 YYYYMMDD
    seg_hour     TEXT    NOT NULL,          -- 小时 HH（按小时分桶）
    template_id  TEXT    DEFAULT '',        -- 模板 ID（空=未聚类）
    line_count   INTEGER DEFAULT 0,         -- 段内总行数
    error_count  INTEGER DEFAULT 0,         -- 段内 ERROR 行数
    warn_count   INTEGER DEFAULT 0,         -- 段内 WARN 行数
    first_ts     TEXT,                      -- 段内首行时间戳
    last_ts      TEXT,                      -- 段内末行时间戳
    compressed   BLOB,                      -- zstd 压缩后的列式数据
    created_time TEXT DEFAULT (datetime('now','localtime')),
    UNIQUE(item_id, client_id, seg_date, seg_hour)
);
CREATE INDEX IF NOT EXISTS idx_segment_lookup ON t_log_segment(item_id, client_id, seg_date);
CREATE INDEX IF NOT EXISTS idx_segment_template ON t_log_segment(template_id);

-- V2.1 新增表：日志模板（Template）
CREATE TABLE IF NOT EXISTS t_log_template (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id   TEXT    NOT NULL UNIQUE,  -- fnv32a hash
    template_text TEXT    NOT NULL,          -- 模板串（参数位用 <*> 占位）
    service       TEXT    DEFAULT '',        -- 所属服务（按 item_id 聚合）
    level         TEXT    DEFAULT '',        -- 模板主要级别
    first_seen    TEXT,                      -- 首次出现时间
    last_seen     TEXT,                      -- 最近出现时间
    total_count   INTEGER DEFAULT 0,         -- 历史累计命中数
    sample_line   TEXT,                      -- 典型样本行
    created_time  TEXT DEFAULT (datetime('now','localtime')),
    updated_time  TEXT DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_template_service ON t_log_template(service);
CREATE INDEX IF NOT EXISTS idx_template_hash ON t_log_template(template_id);

-- V2.1 新增表：embedding 向量索引元数据
CREATE TABLE IF NOT EXISTS t_embedding (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    target_type   TEXT    NOT NULL,         -- 'template' / 'sample'
    target_id     TEXT    NOT NULL,         -- template_id 或 sample 唯一 ID
    model_version TEXT    NOT NULL,         -- 模型版本标识（如 bge-m3-v1）
    vector_dim    INTEGER NOT NULL,         -- 向量维度
    vector_blob   BLOB    NOT NULL,         -- float32 序列化
    created_time  TEXT DEFAULT (datetime('now','localtime')),
    UNIQUE(target_type, target_id, model_version)
);
CREATE INDEX IF NOT EXISTS idx_embedding_target ON t_embedding(target_type, target_id);

-- V2.1 新增表：异常事件
CREATE TABLE IF NOT EXISTS t_anomaly_event (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id  TEXT,                      -- 关联模板 ID
    item_id      INTEGER NOT NULL,
    client_id    INTEGER NOT NULL,
    anomaly_type TEXT    NOT NULL,           -- 'spike' / 'new' / 'disappear'
            -- spike = 突增 / new = 新模板 / disappear = 消失模板
    score        REAL    DEFAULT 0.0,       -- 异常分（Z-Score）
    baseline     REAL    DEFAULT 0.0,       -- 基线值
    actual       REAL    DEFAULT 0.0,       -- 实际值
    window_sec   INTEGER DEFAULT 300,       -- 检测时间窗（秒）
    status       TEXT    DEFAULT 'active',  -- active / acknowledged / closed
    description  TEXT,                      -- 异常描述
    created_time TEXT DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_anomaly_item ON t_anomaly_event(item_id, client_id);
CREATE INDEX IF NOT EXISTS idx_anomaly_status ON t_anomaly_event(status);

-- V2.1 新增表：链路追踪（TraceNode）
CREATE TABLE IF NOT EXISTS t_trace_node (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id     TEXT    NOT NULL,          -- 链路 ID
    span_id      TEXT    NOT NULL,          -- Span ID
    parent_id    TEXT    DEFAULT '',        -- 父 Span ID
    service      TEXT,                      -- 服务名（从日志提取）
    operation    TEXT,                      -- 操作名
    start_ts     TEXT,                      -- 开始时间戳 ISO8601
    duration_ms  INTEGER DEFAULT 0,         -- 耗时毫秒
    status       TEXT    DEFAULT 'ok',      -- ok / error
    tags         TEXT    DEFAULT '{}',      -- JSON map
    created_time TEXT DEFAULT (datetime('now','localtime')),
    UNIQUE(trace_id, span_id)
);
CREATE INDEX IF NOT EXISTS idx_trace_node_trace ON t_trace_node(trace_id);
CREATE INDEX IF NOT EXISTS idx_trace_node_span ON t_trace_node(span_id);

-- V2.1 新增表：案例库
CREATE TABLE IF NOT EXISTS t_case_library (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    case_id      TEXT    NOT NULL UNIQUE,   -- 唯一 case ID
    title        TEXT    NOT NULL,          -- 案例标题
    symptoms     TEXT    DEFAULT '[]',      -- JSON array：症状列表
    root_cause   TEXT,                      -- 根因描述
    solution     TEXT,                      -- 解决方案
    tag_list     TEXT    DEFAULT '',        -- 标签（逗号分隔）
    source       TEXT    DEFAULT 'manual',  -- manual / conversation / feedback
    conv_id      TEXT,                      -- 来源会话 ID
    hit_count    INTEGER DEFAULT 0,         -- 命中次数
    created_by   TEXT,
    created_time TEXT DEFAULT (datetime('now','localtime')),
    updated_time TEXT DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_case_library_source ON t_case_library(source);
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

	// V2.1：WAL 模式支持「多读一写」并发，适当调高连接池。
	// 写操作由 SQLite 自身串行化，读操作可并发执行。
	db.SetMaxIdleConns(4)
	db.SetMaxOpenConns(8)

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

// tryMigrateAddColumns 尝试为已有库补充新加的列（source_type / identity / virtual_parent / labels / channel）。
// SQLite 不支持 ALTER TABLE IF NOT EXISTS；我们捕获 "duplicate column" 错误并忽略。
func tryMigrateAddColumns(db *sqlx.DB) error {
	columns := []struct {
		name    string
		sqlType string
		def     string
		table   string
	}{
		{"source_type", "TEXT", "DEFAULT 'file'", "t_client"},
		{"identity", "TEXT", "DEFAULT ''", "t_client"},
		{"virtual_parent", "TEXT", "DEFAULT ''", "t_client"},
		{"labels", "TEXT", "DEFAULT '{}'", "t_client"},
		{"channel", "TEXT", "DEFAULT 'webhook'", "t_alert_rule"},
		// V2.1 FR-1305：成本配额字段
		{"cost_per_1k", "REAL", "DEFAULT 0", "t_model_capability"},
		{"quota_daily", "INTEGER", "DEFAULT 0", "t_model_capability"},
		{"quota_used", "INTEGER", "DEFAULT 0", "t_model_capability"},
		{"quota_reset_at", "TEXT", "DEFAULT ''", "t_model_capability"},
	}
	for _, col := range columns {
		_, err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s %s %s;",
			col.table, col.name, col.sqlType, col.def,
		))
		if err != nil && !contains(err.Error(), "duplicate column") {
			return fmt.Errorf("add column %s.%s: %w", col.table, col.name, err)
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
