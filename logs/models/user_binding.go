// Package models 子文件：用户-资源行级绑定关系（t_user_binding 表）。
//
// V2.1 FR-6001：多用户行级 RBAC 扩展。
// 通过 user_id + resource_type + resource_id 三元组控制用户对特定资源行的访问权限。
// admin 角色跳过行级校验（在中间件层处理），operator/viewer 必须拥有对应绑定才可访问。
package models

import (
	"context"
	"errors"
	"log"
)

// 资源类型常量。
const (
	// ResourceItem 日志项资源类型。
	ResourceItem = "item"
	// ResourceClient 客户端资源类型。
	ResourceClient = "client"
)

// 哨兵错误变量。
var (
	// ErrForbiddenError 行级访问拒绝哨兵错误。
	ErrForbiddenError = errors.New("access denied: 无资源访问权")
)

// UserBinding 用户-资源绑定关系实体（表 t_user_binding）。
type UserBinding struct {
	// ID 绑定记录唯一标识
	ID int64 `json:"id" db:"id"`
	// UserID 用户 ID
	UserID int64 `json:"userId" db:"user_id"`
	// ResourceType 资源类型：'item' / 'client'
	ResourceType string `json:"resourceType" db:"resource_type"`
	// ResourceID 资源行 ID
	ResourceID int64 `json:"resourceId" db:"resource_id"`
	// CreatedTime 绑定创建时间
	CreatedTime string `json:"createdTime" db:"created_time"`
}

// 建表 DDL（幂等）。V2.1 新增表，使用 CREATE TABLE IF NOT EXISTS。
const userBindingSchema = `
CREATE TABLE IF NOT EXISTS t_user_binding (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL,
    resource_type TEXT    NOT NULL,
    resource_id   INTEGER NOT NULL,
    created_time  TEXT    DEFAULT (datetime('now','localtime')),
    UNIQUE(user_id, resource_type, resource_id)
);
CREATE INDEX IF NOT EXISTS idx_t_user_binding_user ON t_user_binding(user_id, resource_type);
CREATE INDEX IF NOT EXISTS idx_t_user_binding_resource ON t_user_binding(resource_type, resource_id);
`

// CreateUserBindingTable 创建 t_user_binding 表（在 InitDB 之后调用）。
// 适用于不想修改 def.go schema 的场景，确保表存在。
func CreateUserBindingTable() error {
	if DB == nil {
		return errors.New("database not initialized: DB is nil")
	}
	if _, err := DB.Exec(userBindingSchema); err != nil {
		log.Printf("create user binding table err: %v", err)
		return err
	}
	return nil
}

// CreateUserBinding 创建用户-资源绑定。
// INSERT OR IGNORE 保证幂等：重复绑定不报错，直接忽略。
// 返回 error 仅表示真正的数据库错误（非违反 UNIQUE 约束）。
func CreateUserBinding(ctx context.Context, userID int64, resourceType string, resourceID int64) error {
	res, err := DB.ExecContext(ctx,
		`INSERT OR IGNORE INTO t_user_binding (user_id, resource_type, resource_id) VALUES (?, ?, ?)`,
		userID, resourceType, resourceID)
	if err != nil {
		log.Printf("create user binding err: %v", err)
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		log.Printf("create user binding rows affected err: %v", err)
		return err
	}
	if rows == 0 {
		log.Printf("[binding] duplicate binding ignored: user_id=%d resource_type=%s resource_id=%d",
			userID, resourceType, resourceID)
	}
	return nil
}

// DeleteUserBinding 删除用户-资源绑定。
// 按 user_id + resource_type + resource_id 三元组精确删除。
func DeleteUserBinding(ctx context.Context, userID int64, resourceType string, resourceID int64) error {
	if _, err := DB.ExecContext(ctx,
		`DELETE FROM t_user_binding WHERE user_id = ? AND resource_type = ? AND resource_id = ?`,
		userID, resourceType, resourceID); err != nil {
		log.Printf("delete user binding err: %v", err)
		return err
	}
	return nil
}

// ListUserBindings 查询用户的所有绑定记录。
// 按 id 倒序返回。
func ListUserBindings(ctx context.Context, userID int64) ([]UserBinding, error) {
	var bindings []UserBinding
	if err := DB.SelectContext(ctx, &bindings,
		`SELECT * FROM t_user_binding WHERE user_id = ? ORDER BY id DESC`, userID); err != nil {
		log.Printf("list user bindings err: %v", err)
		return nil, err
	}
	return bindings, nil
}

// CheckUserAccess 检查用户是否有权访问指定资源行。
// 返回 true 表示存在绑定关系，false 表示无权。
func CheckUserAccess(ctx context.Context, userID int64, resourceType string, resourceID int64) (bool, error) {
	var count int
	if err := DB.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM t_user_binding WHERE user_id = ? AND resource_type = ? AND resource_id = ?`,
		userID, resourceType, resourceID); err != nil {
		log.Printf("check user access err: %v", err)
		return false, err
	}
	return count > 0, nil
}

// GetUserResourceIDs 查询用户可访问的指定资源类型的全部资源 ID 列表。
// 用于中间件注入到 gin context 中，供后续 handler/filter 做行级校验。
// 返回空切片表示该资源类型下无任何绑定。
func GetUserResourceIDs(ctx context.Context, userID int64, resourceType string) ([]int64, error) {
	var ids []int64
	if err := DB.SelectContext(ctx, &ids,
		`SELECT resource_id FROM t_user_binding WHERE user_id = ? AND resource_type = ?`,
		userID, resourceType); err != nil {
		log.Printf("get user resource ids err: %v", err)
		return nil, err
	}
	if ids == nil {
		return []int64{}, nil
	}
	return ids, nil
}
