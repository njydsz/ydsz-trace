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
    webhook_url    TEXT    NOT NULL,                   -- webhook URL；email 渠道复用为 smtp:// 连接串
    channel        TEXT    DEFAULT 'webhook',          -- 通知渠道：webhook/wecom/dingtalk/feishu/email
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

-- ==================== 模型能力配置表 ====================
DROP TABLE IF EXISTS t_model_capability;
CREATE TABLE t_model_capability (
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

-- 模型能力配置表索引
CREATE INDEX IF NOT EXISTS idx_model_capability_provider ON t_model_capability(provider);
CREATE INDEX IF NOT EXISTS idx_model_capability_enabled ON t_model_capability(enabled);

-- ==================== V2.0 新表：列式段存储 ====================
DROP TABLE IF EXISTS t_log_segment;
CREATE TABLE t_log_segment (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id      INTEGER NOT NULL,
    client_id    INTEGER NOT NULL,
    seg_date     TEXT    NOT NULL,          -- 日期 YYYYMMDD
    seg_hour     TEXT    NOT NULL,          -- 小时 HH
    template_id  TEXT    DEFAULT '',        -- 模板 ID（空=未聚类）
    line_count   INTEGER DEFAULT 0,
    error_count  INTEGER DEFAULT 0,
    warn_count   INTEGER DEFAULT 0,
    first_ts     TEXT,
    last_ts      TEXT,
    compressed   BLOB,                      -- zstd 压缩列式数据
    created_time TEXT DEFAULT (datetime('now','localtime')),
    FOREIGN KEY (item_id)   REFERENCES t_item(id),
    FOREIGN KEY (client_id) REFERENCES t_client(id),
    UNIQUE(item_id, client_id, seg_date, seg_hour)
);
CREATE INDEX IF NOT EXISTS idx_segment_lookup    ON t_log_segment(item_id, client_id, seg_date);
CREATE INDEX IF NOT EXISTS idx_segment_template  ON t_log_segment(template_id);

-- ==================== V2.0 新表：日志模板 ====================
DROP TABLE IF EXISTS t_log_template;
CREATE TABLE t_log_template (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id   TEXT    NOT NULL UNIQUE,  -- fnv32a hash
    template_text TEXT    NOT NULL,          -- 模板串（参数位用 <*> 占位）
    service       TEXT    DEFAULT '',
    level         TEXT    DEFAULT '',
    first_seen    TEXT,
    last_seen     TEXT,
    total_count   INTEGER DEFAULT 0,
    sample_line   TEXT,
    created_time  TEXT DEFAULT (datetime('now','localtime')),
    updated_time  TEXT DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_template_service ON t_log_template(service);
CREATE INDEX IF NOT EXISTS idx_template_hash    ON t_log_template(template_id);

-- ==================== V2.0 新表：embedding 向量索引元数据 ====================
DROP TABLE IF EXISTS t_embedding;
CREATE TABLE t_embedding (
    target_type   TEXT    NOT NULL,         -- 'template' / 'sample'
    target_id     TEXT    NOT NULL,
    model_version TEXT    NOT NULL,
    vector_dim    INTEGER NOT NULL,
    vector_blob   BLOB    NOT NULL,         -- float32 序列化
    created_time  TEXT DEFAULT (datetime('now','localtime')),
    UNIQUE(target_type, target_id, model_version)
);
CREATE INDEX IF NOT EXISTS idx_embedding_target ON t_embedding(target_type, target_id);

-- ==================== V2.0 新表：异常事件 ====================
DROP TABLE IF EXISTS t_anomaly_event;
CREATE TABLE t_anomaly_event (
    template_id  TEXT    DEFAULT '',
    item_id      INTEGER NOT NULL,
    client_id    INTEGER NOT NULL,
    anomaly_type TEXT    NOT NULL,          -- spike / drop / new_pattern
    score        REAL    DEFAULT 0,
    baseline     REAL    DEFAULT 0,
    actual       REAL    DEFAULT 0,
    window_sec   INTEGER DEFAULT 300,
    status       TEXT    DEFAULT 'open',    -- open / acknowledged / resolved
    description  TEXT    DEFAULT '',
    created_time TEXT DEFAULT (datetime('now','localtime')),
    FOREIGN KEY (item_id)   REFERENCES t_item(id),
    FOREIGN KEY (client_id) REFERENCES t_client(id)
);
CREATE INDEX IF NOT EXISTS idx_anomaly_item     ON t_anomaly_event(item_id, client_id);
CREATE INDEX IF NOT EXISTS idx_anomaly_status   ON t_anomaly_event(status);
CREATE INDEX IF NOT EXISTS idx_anomaly_time     ON t_anomaly_event(created_time);

-- ==================== V2.0 新表：链路追踪节点 ====================
DROP TABLE IF EXISTS t_trace_node;
CREATE TABLE t_trace_node (
    trace_id    TEXT    NOT NULL,
    span_id     TEXT    NOT NULL,
    parent_id   TEXT    DEFAULT '',
    service     TEXT    DEFAULT '',
    operation   TEXT    DEFAULT '',
    start_ts    TEXT,
    duration_ms INTEGER DEFAULT 0,
    status      TEXT    DEFAULT '',
    tags        TEXT    DEFAULT '{}',
    created_time TEXT DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_trace_node_trace  ON t_trace_node(trace_id);
CREATE INDEX IF NOT EXISTS idx_trace_node_span   ON t_trace_node(span_id);
CREATE INDEX IF NOT EXISTS idx_trace_node_service ON t_trace_node(service);

-- ==================== V2.0 新表：案例库 ====================
DROP TABLE IF EXISTS t_case_library;
CREATE TABLE t_case_library (
    case_id     TEXT    NOT NULL UNIQUE,
    title       TEXT    NOT NULL,
    symptoms    TEXT    DEFAULT '{}',       -- JSON 数组
    root_cause  TEXT    DEFAULT '',
    solution    TEXT    DEFAULT '',
    tag_list    TEXT    DEFAULT '{}',       -- JSON 数组
    source      TEXT    DEFAULT 'manual',   -- manual / auto
    conv_id     TEXT    DEFAULT '',
    hit_count   INTEGER DEFAULT 0,
    created_by  TEXT    DEFAULT '',
    created_time TEXT DEFAULT (datetime('now','localtime')),
    updated_time TEXT DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_case_tag     ON t_case_library(tag_list);
CREATE INDEX IF NOT EXISTS idx_case_service ON t_case_library(created_by);

-- ==================== V2.2 异常反馈与阈值自适应（FR-807）====================
DROP TABLE IF EXISTS t_anomaly_feedback;
CREATE TABLE t_anomaly_feedback (
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

-- ==================== V2.2 告警静默窗口（FR-806）====================
DROP TABLE IF EXISTS t_maintenance_window;
CREATE TABLE t_maintenance_window (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL,
    start_time   TEXT    NOT NULL,          -- HH:MM
    end_time     TEXT    NOT NULL,          -- HH:MM（支持跨天）
    recurring    TEXT    DEFAULT 'daily',   -- daily / weekly / once
    weekdays     TEXT    DEFAULT '',        -- JSON 数组，weekly 时生效
    enabled      INTEGER DEFAULT 1,
    created_time TEXT DEFAULT (datetime('now','localtime')),
    updated_time TEXT DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_maint_window_enabled ON t_maintenance_window(enabled);

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
