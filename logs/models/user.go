// Package models 子文件：多用户管理（t_user 表）。
//
// 支持多账号登录：密码使用 argon2id 哈希后存储，
// role 字段控制权限（admin / operator / viewer），
// status 字段控制启用/禁用状态。
package models

import (
	"log"
	"time"
)

// StatusEnabled / StatusDisabled 用户启用状态常量。
const (
	StatusEnabled  = 1
	StatusDisabled = 0
)

// TUser 用户实体（表 t_user）。
type TUser struct {
	// ID 用户唯一标识
	ID int64 `json:"id" db:"id"`
	// Username 用户名（唯一）
	Username string `json:"username" db:"username"`
	// PasswordHash 密码哈希（argon2id 格式），JSON 序列化时隐藏
	PasswordHash string `json:"-" db:"password_hash"`
	// Role 用户角色：admin / operator / viewer
	Role string `json:"role" db:"role"`
	// Status 状态：1-启用 0-禁用
	Status int `json:"status" db:"status"`
	// CreatedTime 创建时间
	CreatedTime string `json:"createdTime" db:"created_time"`
	// UpdatedTime 更新时间
	UpdatedTime string `json:"updatedTime" db:"updated_time"`
}

// AddUser 创建新用户，密码必须先经过 argon2id 哈希后传入 PasswordHash。
func AddUser(user *TUser) (int64, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	if user.CreatedTime == "" {
		user.CreatedTime = now
	}
	if user.UpdatedTime == "" {
		user.UpdatedTime = now
	}
	if user.Role == "" {
		user.Role = "viewer"
	}
	if user.Status == 0 {
		user.Status = StatusEnabled
	}
	res, err := DB.Exec(`INSERT INTO t_user (username, password_hash, role, status, created_time, updated_time)
		VALUES (?, ?, ?, ?, ?, ?)`,
		user.Username, user.PasswordHash, user.Role, user.Status, user.CreatedTime, user.UpdatedTime)
	if err != nil {
		log.Printf("insert user err: %v", err)
		return 0, err
	}
	return res.LastInsertId()
}

// GetUserByUsername 按用户名查询单个用户。
func GetUserByUsername(username string) (TUser, error) {
	var user TUser
	err := DB.Get(&user, `SELECT * FROM t_user WHERE username = ?`, username)
	if err != nil {
		log.Printf("get user by username err: %v", err)
	}
	return user, err
}

// GetUserByID 按 id 查询单个用户。
func GetUserByID(id int64) (TUser, error) {
	var user TUser
	err := DB.Get(&user, `SELECT * FROM t_user WHERE id = ?`, id)
	if err != nil {
		log.Printf("get user by id err: %v", err)
	}
	return user, err
}

// UpdateUser 更新用户信息（不含密码）。
func UpdateUser(user *TUser) error {
	user.UpdatedTime = time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE t_user SET role = ?, status = ?, updated_time = ? WHERE id = ?`,
		user.Role, user.Status, user.UpdatedTime, user.ID)
	if err != nil {
		log.Printf("update user err: %v", err)
	}
	return err
}

// UpdateUserPassword 更新用户密码哈希。
func UpdateUserPassword(id int64, passwordHash string) error {
	_, err := DB.Exec(`UPDATE t_user SET password_hash = ?, updated_time = ? WHERE id = ?`,
		passwordHash, time.Now().Format("2006-01-02 15:04:05"), id)
	if err != nil {
		log.Printf("update user password err: %v", err)
	}
	return err
}

// DeleteUser 禁用用户（软禁用：设置 status=0，不物理删除）。
func DeleteUser(id int64) error {
	_, err := DB.Exec(`UPDATE t_user SET status = ?, updated_time = ? WHERE id = ?`,
		StatusDisabled, time.Now().Format("2006-01-02 15:04:05"), id)
	if err != nil {
		log.Printf("disable user err: %v", err)
	}
	return err
}

// QueryUsers 分页查询用户列表（按 id 倒序）。
// keyword 过滤 username。
func QueryUsers(pageNo, pageSize int, keyword string) Page {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	where := ""
	args := []interface{}{}
	if keyword != "" {
		where = "WHERE username LIKE ?"
		args = append(args, "%"+keyword+"%")
	}
	var users []TUser
	err := DB.Select(&users,
		`SELECT * FROM t_user `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (pageNo-1)*pageSize)...)
	if err != nil {
		log.Printf("query users err: %v", err)
		return Page{}
	}
	var total int
	err = DB.Get(&total, `SELECT COUNT(*) FROM t_user `+where, args...)
	if err != nil {
		log.Printf("count users err: %v", err)
		return Page{}
	}
	return PageUtil(total, pageNo, pageSize, users)
}

// UserExists 判断用户名是否已存在（用于创建时唯一性校验，排除指定 id）。
func UserExists(username string, excludeID int64) bool {
	var count int
	if excludeID > 0 {
		DB.Get(&count, `SELECT COUNT(*) FROM t_user WHERE username = ? AND id != ?`, username, excludeID)
	} else {
		DB.Get(&count, `SELECT COUNT(*) FROM t_user WHERE username = ?`, username)
	}
	return count > 0
}
