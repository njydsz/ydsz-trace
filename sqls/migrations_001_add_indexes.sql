-- ============================================================
-- Migration 001: 添加性能优化索引
-- 日期: 2026-08-08
-- 数据库: MySQL
-- 描述: 为高频查询添加联合索引和单列索引
-- ============================================================

-- ---------- MySQL ----------
-- 客户端表：按 ip + port + vkey 联合唯一校验（高频查询）
CREATE UNIQUE INDEX IF NOT EXISTS idx_client_ip_port_vkey ON t_client(ip, port, vkey);

-- 客户端表：按 online 状态筛选（定时任务场景）
CREATE INDEX IF NOT EXISTS idx_client_online ON t_client(online);

-- 客户端表：按 status 状态筛选
CREATE INDEX IF NOT EXISTS idx_client_status ON t_client(status);

-- 项目表：按 client_id 查询（高频连接查询）
CREATE INDEX IF NOT EXISTS idx_item_client_id ON t_item(client_id);

-- 项目表：按 status 状态筛选
CREATE INDEX IF NOT EXISTS idx_item_status ON t_item(status);

-- 项目表：按项目名称搜索
CREATE INDEX IF NOT EXISTS idx_item_name ON t_item(item_name);
