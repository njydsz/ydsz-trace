-- ============================================================
-- ydsz-trace SQLite 数据库初始化脚本
-- 适用于 SQLite 3.35+ (modernc.org/sqlite 纯 Go 驱动)
-- WAL 模式 + 外键约束，自动建表由程序 InitDB 完成，本脚本仅用于手动初始化
-- 与 logs/models/def.go 中的 schema 常量保持一致
-- ============================================================

-- ==================== 客户端表 ====================
DROP TABLE IF EXISTS t_client;
CREATE TABLE t_client (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    ip             TEXT,                       -- IP
    port           TEXT,                       -- Port
    vkey           TEXT,                       -- 密钥
    info           TEXT,                       -- 备注
    zip            TEXT    DEFAULT '1',        -- 压缩 0-不压缩 1-压缩
    status         TEXT    DEFAULT '1',        -- 状态 0-无效 1-有效
    online         TEXT    DEFAULT '0',        -- 在线 0-离线 1-在线
    source_type    TEXT    DEFAULT 'file',     -- 来源类型：file / docker / k8s
    identity       TEXT    DEFAULT '',         -- 虚拟客户端稳定标识（容器 ID 等）
    virtual_parent TEXT    DEFAULT '',         -- 虚拟父客户端标识（同 logc 多实例）
    labels         TEXT    DEFAULT '{}',       -- JSON 字符串，存储扩展标签
    created_by     TEXT,                       -- 创建人
    created_time   TEXT    DEFAULT (datetime('now', 'localtime')),  -- 创建时间
    updated_by     TEXT,                       -- 更新人
    updated_time   TEXT    DEFAULT (datetime('now', 'localtime'))   -- 更新时间
);

-- 客户端表索引
CREATE INDEX IF NOT EXISTS idx_t_client_ip_port ON t_client(ip, port);
CREATE INDEX IF NOT EXISTS idx_t_client_source ON t_client(source_type);
CREATE INDEX IF NOT EXISTS idx_t_client_identity ON t_client(identity);
CREATE INDEX IF NOT EXISTS idx_t_client_virtual_parent ON t_client(virtual_parent);

-- ==================== 项目表 ====================
DROP TABLE IF EXISTS t_item;
CREATE TABLE t_item (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id    INTEGER NOT NULL,
    item_name    TEXT,                       -- 项目名称
    item_desc    TEXT,                       -- 项目描述
    log_path     TEXT,                       -- 日志路径
    log_prefix   TEXT,                       -- 日志前缀
    log_suffix   TEXT,                       -- 日志后缀
    status       TEXT    DEFAULT '1',        -- 状态 0-无效 1-有效
    created_by   TEXT,                       -- 创建人
    created_time TEXT    DEFAULT (datetime('now', 'localtime')),  -- 创建时间
    updated_by   TEXT,                       -- 更新人
    updated_time TEXT    DEFAULT (datetime('now', 'localtime')),  -- 更新时间
    FOREIGN KEY (client_id) REFERENCES t_client(id)
);

-- 项目表索引
CREATE INDEX IF NOT EXISTS idx_t_item_client_id ON t_item(client_id);

-- ==================== 检索任务表 ====================
DROP TABLE IF EXISTS t_task;
CREATE TABLE t_task (
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

-- 检索任务表索引
CREATE INDEX IF NOT EXISTS idx_t_task_status ON t_task(status);
CREATE INDEX IF NOT EXISTS idx_t_task_created ON t_task(created_time);

-- ==================== 告警规则表 ====================
DROP TABLE IF EXISTS t_alert_rule;
CREATE TABLE t_alert_rule (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT    NOT NULL,
    item_id        INTEGER NOT NULL,
    client_id      INTEGER NOT NULL DEFAULT 0,         -- 0 表示所有客户端
    key_word       TEXT    DEFAULT '',
    regex          INTEGER DEFAULT 0,                  -- 0 = 包含匹配，1 = 正则
    level          TEXT    DEFAULT '',
    threshold      INTEGER DEFAULT 1,                  -- 命中 >= threshold 才触发
    interval_sec   INTEGER DEFAULT 300,                -- 评估间隔（秒），同时用作周期去重窗口
    webhook_url    TEXT    NOT NULL,
    enabled        INTEGER DEFAULT 1,
    last_fired     TEXT    DEFAULT '',
    created_by     TEXT    DEFAULT '',
    created_time   TEXT    DEFAULT (datetime('now', 'localtime')),
    updated_time   TEXT    DEFAULT (datetime('now', 'localtime')),
    FOREIGN KEY (item_id) REFERENCES t_item(id)
);

-- 告警规则表索引
CREATE INDEX IF NOT EXISTS idx_alert_rule_item ON t_alert_rule(item_id);
CREATE INDEX IF NOT EXISTS idx_alert_rule_enabled ON t_alert_rule(enabled);

-- ==================== 告警投递记录表 ====================
DROP TABLE IF EXISTS t_user;
CREATE TABLE t_user (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    role          TEXT    DEFAULT 'viewer',
    status        INTEGER DEFAULT 1,
    created_time  TEXT    DEFAULT (datetime('now', 'localtime')),
    updated_time  TEXT    DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_t_user_username ON t_user(username);

DROP TABLE IF EXISTS t_alert_event;
CREATE TABLE t_alert_event (
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

-- 告警投递记录表索引
CREATE INDEX IF NOT EXISTS idx_alert_event_rule ON t_alert_event(rule_id);
CREATE INDEX IF NOT EXISTS idx_alert_event_time ON t_alert_event(fired_time);

-- ==================== 模式时序计数表 ====================
DROP TABLE IF EXISTS t_pattern_stat;
CREATE TABLE t_pattern_stat (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id      INTEGER NOT NULL,           -- 日志项 ID
    client_id    INTEGER NOT NULL,           -- 客户端 ID
    template_hash TEXT   NOT NULL,           -- 模板模式 hash（normalized line 的 hash）
    time_bucket  TEXT    NOT NULL,           -- 时间桶（YYYYMMDDHHMI，按 5 分钟对齐）
    count        INTEGER DEFAULT 0,          -- 命中行数
    first_time   TEXT,                       -- 首次出现时间
    last_time    TEXT,                       -- 末次出现时间
    sample       TEXT,                       -- 第一条命中行原文
    created_time TEXT    DEFAULT (datetime('now','localtime')),
    UNIQUE(item_id, client_id, template_hash, time_bucket)
);

-- 模式时序计数表索引
CREATE INDEX IF NOT EXISTS idx_pattern_stat_lookup ON t_pattern_stat(item_id, client_id, time_bucket);
CREATE INDEX IF NOT EXISTS idx_pattern_stat_template ON t_pattern_stat(template_hash);

-- ==================== 审计日志表 ====================
DROP TABLE IF EXISTS t_audit_log;
CREATE TABLE t_audit_log (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    username     TEXT,                       -- 操作用户
    action       TEXT    NOT NULL,           -- 操作类型：search/export/login/config_change/delete
    resource     TEXT,                       -- 操作对象：/logs/search, /client/register 等
    detail       TEXT,                       -- JSON 格式详情：搜索条件、影响行数等
    client_ip    TEXT,                       -- 请求方 IP
    result       TEXT    DEFAULT 'success',  -- success/fail
    created_time TEXT    DEFAULT (datetime('now', 'localtime'))
);

-- 审计日志表索引
CREATE INDEX IF NOT EXISTS idx_audit_username ON t_audit_log(username);
CREATE INDEX IF NOT EXISTS idx_audit_action ON t_audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_time ON t_audit_log(created_time);

-- ==================== SQLite 性能优化 PRAGMA ====================
-- WAL 模式：提升并发读写性能
PRAGMA journal_mode = WAL;
-- 同步模式：NORMAL 在 WAL 模式下足够安全
PRAGMA synchronous = NORMAL;
-- 外键约束
PRAGMA foreign_keys = ON;
