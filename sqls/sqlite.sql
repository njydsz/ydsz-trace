-- ============================================================
-- ydsz-trace SQLite 数据库初始化脚本
-- 适用于 SQLite 3.35+ (modernc.org/sqlite 纯 Go 驱动)
-- WAL 模式 + 外键约束，自动建表由程序 InitDB 完成，本脚本仅用于手动初始化
-- ============================================================

-- 删除客户端表
DROP TABLE IF EXISTS t_client;
-- 创建客户端表
CREATE TABLE t_client (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ip          TEXT,                       -- IP
    port        TEXT,                       -- Port
    vkey        TEXT,                       -- 密钥
    info        TEXT,                       -- 备注
    zip         TEXT    DEFAULT '1',        -- 压缩 0-不压缩 1-压缩
    status      TEXT    DEFAULT '1',        -- 状态 0-无效 1-有效
    online      TEXT    DEFAULT '0',        -- 在线 0-离线 1-在线
    created_by  TEXT,                       -- 创建人
    created_time TEXT   DEFAULT (datetime('now', 'localtime')),  -- 创建时间
    updated_by  TEXT,                       -- 更新人
    updated_time TEXT   DEFAULT (datetime('now', 'localtime'))   -- 更新时间
);

-- 客户端表索引
CREATE INDEX IF NOT EXISTS idx_t_client_ip_port ON t_client(ip, port);
CREATE INDEX IF NOT EXISTS idx_t_client_status ON t_client(status);

-- 删除项目表
DROP TABLE IF EXISTS t_item;
-- 创建项目表
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
CREATE INDEX IF NOT EXISTS idx_t_item_status ON t_item(status);

-- SQLite 性能优化 PRAGMA（启动时自动执行）
-- WAL 模式：提升并发读写性能
PRAGMA journal_mode = WAL;
-- 同步模式：NORMAL 在 WAL 模式下足够安全
PRAGMA synchronous = NORMAL;
-- 外键约束
PRAGMA foreign_keys = ON;
