// Package models 行级绑定关系测试（V2.1 FR-6001）。
package models

import (
	"context"
	"path/filepath"
	"testing"

	"ydsz-trace/pkg/auth"
)

// setupBindingTestDB 初始化测试数据库并确保 t_user_binding 表存在。
func setupBindingTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_binding.db")
	cfg := &SQLiteConfig{FilePath: dbPath}
	if err := InitDB(cfg); err != nil {
		t.Fatalf("初始化测试数据库失败: %v", err)
	}
	if err := CreateUserBindingTable(); err != nil {
		t.Fatalf("创建 t_user_binding 表失败: %v", err)
	}
	t.Cleanup(func() {
		if DB != nil {
			_ = DB.Close()
		}
	})
}

// insertTestUser 创建测试用户并返回 user_id。
func insertTestUser(t *testing.T, username, role string) int64 {
	t.Helper()
	hash, err := auth.HashPassword("testpwd123")
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}
	id, err := AddUser(&TUser{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Status:       StatusEnabled,
	})
	if err != nil {
		t.Fatalf("insert user failed: %v", err)
	}
	return id
}

// TestCreatebinding_DuplicateIgnored 验证重复绑定被 UNIQUE 约束静默忽略。
func TestCreatebinding_DuplicateIgnored(t *testing.T) {
	setupBindingTestDB(t)

	ctx := context.Background()
	userID := int64(1001)

	// 首次绑定应成功
	err := CreateUserBinding(ctx, userID, ResourceItem, 100)
	if err != nil {
		t.Fatalf("首次创建绑定失败: %v", err)
	}

	// 重复绑定应被忽略，不返回错误
	err = CreateUserBinding(ctx, userID, ResourceItem, 100)
	if err != nil {
		t.Fatalf("重复绑定应被忽略，但返回了错误: %v", err)
	}

	// 验证只有一条记录
	ids, err := GetUserResourceIDs(ctx, userID, ResourceItem)
	if err != nil {
		t.Fatalf("查询绑定失败: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("期望绑定数为 1，实际 %d", len(ids))
	}
	if ids[0] != 100 {
		t.Errorf("期望 resource_id=100，实际 %d", ids[0])
	}
}

// TestGetUserResourceIDs_Empty 验证无绑定用户返回空列表。
func TestGetUserResourceIDs_Empty(t *testing.T) {
	setupBindingTestDB(t)

	ctx := context.Background()
	userID := int64(2001)

	ids, err := GetUserResourceIDs(ctx, userID, ResourceItem)
	if err != nil {
		t.Fatalf("查询空绑定列表失败: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("期望空列表，实际返回 %d 条", len(ids))
	}
}

// TestCheckUserAccess_Forbidden 验证未授权用户无法访问绑定资源。
func TestCheckUserAccess_Forbidden(t *testing.T) {
	setupBindingTestDB(t)

	ctx := context.Background()
	ownerID := insertTestUser(t, "owner_user", "operator")
	otherID := insertTestUser(t, "other_user", "viewer")

	// owner 创建资源绑定
	err := CreateUserBinding(ctx, ownerID, ResourceItem, 500)
	if err != nil {
		t.Fatalf("创建 owner 绑定失败: %v", err)
	}

	// owner 有权访问
	ok, err := CheckUserAccess(ctx, ownerID, ResourceItem, 500)
	if err != nil {
		t.Fatalf("检查 owner 权限失败: %v", err)
	}
	if !ok {
		t.Error("owner 应有资源 500 的访问权")
	}

	// other_user 无权访问
	ok, err = CheckUserAccess(ctx, otherID, ResourceItem, 500)
	if err != nil {
		t.Fatalf("检查 other 权限失败: %v", err)
	}
	if ok {
		t.Error("other_user 不应有资源 500 的访问权，期望返回 false")
	}
}
