package models

import (
	"path/filepath"
	"testing"
)

// setupQuotaTestDB 初始化临时测试数据库。
func setupQuotaTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_quota.db")
	cfg := &SQLiteConfig{FilePath: dbPath}
	if err := InitDB(cfg); err != nil {
		t.Fatalf("初始化测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if DB != nil {
			_ = DB.Close()
		}
	})
}

// TestCheckQuotaAndAlert_NoConfig 无配置时不限制。
func TestCheckQuotaAndAlert_NoConfig(t *testing.T) {
	setupQuotaTestDB(t)
	ok, remaining, usage, err := CheckQuotaAndAlert("nonexistent-model", 1000)
	if err != nil {
		t.Fatalf("CheckQuotaAndAlert 失败: %v", err)
	}
	if !ok {
		t.Error("无配置时应可用")
	}
	if remaining != 0 {
		t.Errorf("remaining = %d want 0", remaining)
	}
	if usage != 0 {
		t.Errorf("usage = %d want 0", usage)
	}
}

// TestCheckQuotaAndAlert_BelowThreshold 低于阈值不告警。
func TestCheckQuotaAndAlert_BelowThreshold(t *testing.T) {
	setupQuotaTestDB(t)

	// 插入模型配置：配额 10000，已用 5000（50%），阈值 80%。
	_, err := DB.Exec(`
		INSERT INTO t_model_capability (name, provider, model_id, capability, cost_per_1k, quota_daily, quota_used, quota_alert_threshold)
		VALUES ('test-model', 'openai', 'test-gpt', 'chat', 0.01, 10000, 5000, 80)
	`)
	if err != nil {
		t.Fatalf("插入模型配置失败: %v", err)
	}

	ok, remaining, usage, err := CheckQuotaAndAlert("test-gpt", 1000)
	if err != nil {
		t.Fatalf("CheckQuotaAndAlert 失败: %v", err)
	}
	if !ok {
		t.Error("50% 使用率应可用")
	}
	if usage != 50 {
		t.Errorf("usage = %d want 50", usage)
	}
	if remaining != 5000 {
		t.Errorf("remaining = %d want 5000", remaining)
	}
}

// TestCheckQuotaAndAlert_AtThreshold 达到阈值触发告警。
func TestCheckQuotaAndAlert_AtThreshold(t *testing.T) {
	setupQuotaTestDB(t)

	// 插入模型配置：配额 10000，已用 8000（80%），阈值 80%。
	_, err := DB.Exec(`
		INSERT INTO t_model_capability (name, provider, model_id, capability, cost_per_1k, quota_daily, quota_used, quota_alert_threshold)
		VALUES ('test-model', 'openai', 'test-gpt', 'chat', 0.01, 10000, 8000, 80)
	`)
	if err != nil {
		t.Fatalf("插入模型配置失败: %v", err)
	}

	ok, remaining, usage, err := CheckQuotaAndAlert("test-gpt", 1000)
	if err != nil {
		t.Fatalf("CheckQuotaAndAlert 失败: %v", err)
	}
	if !ok {
		t.Error("80% 使用率应仍可用（未超限）")
	}
	if usage != 80 {
		t.Errorf("usage = %d want 80", usage)
	}
	if remaining != 2000 {
		t.Errorf("remaining = %d want 2000", remaining)
	}

	// 验证告警事件已写入。
	var count int
	if err := DB.Get(&count, `SELECT COUNT(*) FROM t_alert_event WHERE rule_name = '配额告警-test-gpt'`); err != nil {
		t.Fatalf("查询告警事件失败: %v", err)
	}
	if count != 1 {
		t.Errorf("告警事件数 = %d want 1", count)
	}
}

// TestCheckQuotaAndAlert_Exceeded 超限不可用。
func TestCheckQuotaAndAlert_Exceeded(t *testing.T) {
	setupQuotaTestDB(t)

	// 插入模型配置：配额 10000，已用 9500（95%），阈值 80%。
	_, err := DB.Exec(`
		INSERT INTO t_model_capability (name, provider, model_id, capability, cost_per_1k, quota_daily, quota_used, quota_alert_threshold)
		VALUES ('test-model', 'openai', 'test-gpt', 'chat', 0.01, 10000, 9500, 80)
	`)
	if err != nil {
		t.Fatalf("插入模型配置失败: %v", err)
	}

	ok, _, usage, err := CheckQuotaAndAlert("test-gpt", 1000)
	if err != nil {
		t.Fatalf("CheckQuotaAndAlert 失败: %v", err)
	}
	if ok {
		t.Error("95% 使用率 + 1000 tokens 应超限")
	}
	if usage != 95 {
		t.Errorf("usage = %d want 95", usage)
	}
}

// TestCheckQuotaAndAlert_NoDuplicate 24h 内不重复告警。
func TestCheckQuotaAndAlert_NoDuplicate(t *testing.T) {
	setupQuotaTestDB(t)

	// 插入模型配置：配额 10000，已用 8000（80%），阈值 80%。
	_, err := DB.Exec(`
		INSERT INTO t_model_capability (name, provider, model_id, capability, cost_per_1k, quota_daily, quota_used, quota_alert_threshold)
		VALUES ('test-model', 'openai', 'test-gpt', 'chat', 0.01, 10000, 8000, 80)
	`)
	if err != nil {
		t.Fatalf("插入模型配置失败: %v", err)
	}

	// 第一次检查：应触发告警。
	_, _, _, err = CheckQuotaAndAlert("test-gpt", 0)
	if err != nil {
		t.Fatalf("第一次检查失败: %v", err)
	}

	// 第二次检查：不应重复告警。
	_, _, _, err = CheckQuotaAndAlert("test-gpt", 0)
	if err != nil {
		t.Fatalf("第二次检查失败: %v", err)
	}

	var count int
	if err := DB.Get(&count, `SELECT COUNT(*) FROM t_alert_event WHERE rule_name = '配额告警-test-gpt'`); err != nil {
		t.Fatalf("查询告警事件失败: %v", err)
	}
	if count != 1 {
		t.Errorf("告警事件数 = %d want 1（24h 内不重复）", count)
	}
}

// TestConsumeQuotaAndAlert 消费并触发告警。
func TestConsumeQuotaAndAlert(t *testing.T) {
	setupQuotaTestDB(t)

	// 插入模型配置：配额 10000，已用 7900（79%），阈值 80%。
	_, err := DB.Exec(`
		INSERT INTO t_model_capability (name, provider, model_id, capability, cost_per_1k, quota_daily, quota_used, quota_alert_threshold)
		VALUES ('test-model', 'openai', 'test-gpt', 'chat', 0.01, 10000, 7900, 80)
	`)
	if err != nil {
		t.Fatalf("插入模型配置失败: %v", err)
	}

	// 消费 200 tokens → 使用率 81%，达到阈值。
	if err := ConsumeQuotaAndAlert("test-gpt", 200); err != nil {
		t.Fatalf("ConsumeQuotaAndAlert 失败: %v", err)
	}

	// 验证配额已更新。
	var quotaUsed int64
	if err := DB.Get(&quotaUsed, `SELECT quota_used FROM t_model_capability WHERE model_id = 'test-gpt'`); err != nil {
		t.Fatalf("查询配额失败: %v", err)
	}
	if quotaUsed != 8100 {
		t.Errorf("quota_used = %d want 8100", quotaUsed)
	}

	// 验证告警事件已写入。
	var count int
	if err := DB.Get(&count, `SELECT COUNT(*) FROM t_alert_event WHERE rule_name = '配额告警-test-gpt'`); err != nil {
		t.Fatalf("查询告警事件失败: %v", err)
	}
	if count != 1 {
		t.Errorf("告警事件数 = %d want 1", count)
	}
}

// TestCheckQuotaAndAlert_CrossDayReset 跨天重置配额。
func TestCheckQuotaAndAlert_CrossDayReset(t *testing.T) {
	setupQuotaTestDB(t)

	// 插入模型配置：配额 10000，已用 9000（90%），阈值 80%，重置时间为 2 天前。
	_, err := DB.Exec(`
		INSERT INTO t_model_capability (name, provider, model_id, capability, cost_per_1k, quota_daily, quota_used, quota_reset_at, quota_alert_threshold)
		VALUES ('test-model', 'openai', 'test-gpt', 'chat', 0.01, 10000, 9000, '2026-11-04 00:00:00', 80)
	`)
	if err != nil {
		t.Fatalf("插入模型配置失败: %v", err)
	}

	ok, _, usage, err := CheckQuotaAndAlert("test-gpt", 0)
	if err != nil {
		t.Fatalf("CheckQuotaAndAlert 失败: %v", err)
	}
	if !ok {
		t.Error("跨天重置后应可用")
	}
	if usage != 0 {
		t.Errorf("usage = %d want 0（已重置）", usage)
	}
}

// TestCheckQuotaAndAlert_ZeroQuota 零配额（不限）不告警。
func TestCheckQuotaAndAlert_ZeroQuota(t *testing.T) {
	setupQuotaTestDB(t)

	// 插入模型配置：配额 0（不限），已用 999999。
	_, err := DB.Exec(`
		INSERT INTO t_model_capability (name, provider, model_id, capability, cost_per_1k, quota_daily, quota_used, quota_alert_threshold)
		VALUES ('test-model', 'openai', 'test-gpt', 'chat', 0.01, 0, 999999, 80)
	`)
	if err != nil {
		t.Fatalf("插入模型配置失败: %v", err)
	}

	ok, _, usage, err := CheckQuotaAndAlert("test-gpt", 1000)
	if err != nil {
		t.Fatalf("CheckQuotaAndAlert 失败: %v", err)
	}
	if !ok {
		t.Error("零配额应可用")
	}
	if usage != 0 {
		t.Errorf("usage = %d want 0（零配额不计算使用率）", usage)
	}
}

// TestCheckQuotaAndAlert_DefaultThreshold 默认阈值 80。
func TestCheckQuotaAndAlert_DefaultThreshold(t *testing.T) {
	setupQuotaTestDB(t)

	// 插入模型配置：不指定阈值（默认 80）。
	_, err := DB.Exec(`
		INSERT INTO t_model_capability (name, provider, model_id, capability, cost_per_1k, quota_daily, quota_used)
		VALUES ('test-model', 'openai', 'test-gpt', 'chat', 0.01, 10000, 8000)
	`)
	if err != nil {
		t.Fatalf("插入模型配置失败: %v", err)
	}

	_, _, usage, err := CheckQuotaAndAlert("test-gpt", 0)
	if err != nil {
		t.Fatalf("CheckQuotaAndAlert 失败: %v", err)
	}
	if usage != 80 {
		t.Errorf("usage = %d want 80", usage)
	}

	// 验证默认阈值下也触发了告警。
	var count int
	if err := DB.Get(&count, `SELECT COUNT(*) FROM t_alert_event WHERE rule_name = '配额告警-test-gpt'`); err != nil {
		t.Fatalf("查询告警事件失败: %v", err)
	}
	if count != 1 {
		t.Errorf("默认阈值下告警事件数 = %d want 1", count)
	}
}
