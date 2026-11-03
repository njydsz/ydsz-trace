// Package models 子文件：AI 诊断会话持久化（t_conversation / t_conversation_msg）。
//
// V2.1 PRD 7.1：对话式工作台的多轮对话上下文存储。
// 每次对话创建一个 t_conversation 记录，每轮消息写入 t_conversation_msg。
// 支持会话列表查询、历史消息回溯、会话续聊。
package models

import (
	"context"
	"fmt"
	"log"
	"time"
)

// TConversation AI 诊断会话实体（表 t_conversation）。
type TConversation struct {
	Id           int64  `json:"id" db:"id"`
	ConvId       string `json:"convId" db:"conv_id"`
	UserId       int64  `json:"userId" db:"user_id"`
	Username     string `json:"username" db:"username"`
	Title        string `json:"title" db:"title"`
	TokenUsed    int    `json:"tokenUsed" db:"token_used"`
	RoleCount    int    `json:"roleCount" db:"role_count"`
	Status       string `json:"status" db:"status"`
	CreatedTime  string `json:"createdTime" db:"created_time"`
	UpdatedTime  string `json:"updatedTime" db:"updated_time"`
}

// TConversationMsg AI 诊断消息实体（表 t_conversation_msg）。
type TConversationMsg struct {
	Id           int64  `json:"id" db:"id"`
	ConvId       string `json:"convId" db:"conv_id"`
	Turn         int    `json:"turn" db:"turn"`
	Role         string `json:"role" db:"role"`                 // user / assistant / system
	Content      string `json:"content" db:"content"`
	ToolCalls    string `json:"toolCalls" db:"tool_calls"`      // JSON array
	EvidenceRefs string `json:"evidenceRefs" db:"evidence_refs"` // JSON array
	TokenCount   int    `json:"tokenCount" db:"token_count"`
	CreatedTime  string `json:"createdTime" db:"created_time"`
}

// conversationSchema 建表 DDL（幂等）。
const conversationSchema = `
CREATE TABLE IF NOT EXISTS t_conversation (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    conv_id       TEXT    NOT NULL UNIQUE,
    user_id       INTEGER NOT NULL DEFAULT 0,
    username      TEXT    DEFAULT '',
    title         TEXT    DEFAULT '',
    token_used    INTEGER DEFAULT 0,
    role_count    INTEGER DEFAULT 0,
    status        TEXT    DEFAULT 'active',
    created_time  TEXT    DEFAULT (datetime('now', 'localtime')),
    updated_time  TEXT    DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_conversation_user ON t_conversation(user_id);
CREATE INDEX IF NOT EXISTS idx_conversation_updated ON t_conversation(updated_time);

CREATE TABLE IF NOT EXISTS t_conversation_msg (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    conv_id       TEXT    NOT NULL,
    turn          INTEGER NOT NULL,
    role          TEXT    NOT NULL,
    content       TEXT    DEFAULT '',
    tool_calls    TEXT    DEFAULT '[]',
    evidence_refs TEXT    DEFAULT '[]',
    token_count   INTEGER DEFAULT 0,
    created_time  TEXT    DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_conv_msg_conv ON t_conversation_msg(conv_id);
CREATE INDEX IF NOT EXISTS idx_conv_msg_turn ON t_conversation_msg(conv_id, turn);
`

// CreateConversationTable 创建会话相关表（幂等）。
func CreateConversationTable() error {
	if DB == nil {
		return fmt.Errorf("database not initialized: DB is nil")
	}
	if _, err := DB.Exec(conversationSchema); err != nil {
		return fmt.Errorf("CreateConversationTable failed: %w", err)
	}
	return nil
}

// ---- 会话 CRUD ----

// CreateConversation 创建新会话。
func CreateConversation(conv *TConversation) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	if conv.CreatedTime == "" {
		conv.CreatedTime = now
	}
	conv.UpdatedTime = now
	if conv.Status == "" {
		conv.Status = "active"
	}

	res, err := DB.Exec(`INSERT INTO t_conversation
		(conv_id, user_id, username, title, token_used, role_count, status, created_time, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conv.ConvId, conv.UserId, conv.Username, conv.Title,
		conv.TokenUsed, conv.RoleCount, conv.Status, conv.CreatedTime, conv.UpdatedTime)
	if err != nil {
		return fmt.Errorf("CreateConversation failed: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("CreateConversation: get last insert id failed: %w", err)
	}
	conv.Id = id
	return nil
}

// GetConversationByConvID 按 conv_id 查询会话。
func GetConversationByConvID(convID string) (*TConversation, error) {
	var conv TConversation
	if err := DB.Get(&conv,
		`SELECT id, conv_id, user_id, username, title, token_used, role_count, status, created_time, updated_time
		 FROM t_conversation WHERE conv_id = ?`, convID); err != nil {
		return nil, fmt.Errorf("GetConversationByConvID failed: %w", err)
	}
	return &conv, nil
}

// ListConversations 按用户分页查询会话列表（按 updated_time 倒序）。
func ListConversations(userID int64, pageNo, pageSize int) (*Page, error) {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (pageNo - 1) * pageSize

	var count int
	if err := DB.Get(&count, `SELECT COUNT(*) FROM t_conversation WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("ListConversations count failed: %w", err)
	}

	var list []TConversation
	if err := DB.Select(&list,
		`SELECT id, conv_id, user_id, username, title, token_used, role_count, status, created_time, updated_time
		 FROM t_conversation WHERE user_id = ? ORDER BY updated_time DESC LIMIT ? OFFSET ?`,
		userID, pageSize, offset); err != nil {
		return nil, fmt.Errorf("ListConversations query failed: %w", err)
	}

	page := PageUtil(count, pageNo, pageSize, list)
	return &page, nil
}

// UpdateConversationTitle 更新会话标题。
func UpdateConversationTitle(convID, title string) error {
	_, err := DB.Exec(`UPDATE t_conversation SET title = ?, updated_time = ? WHERE conv_id = ?`,
		title, time.Now().Format("2006-01-02 15:04:05"), convID)
	if err != nil {
		return fmt.Errorf("UpdateConversationTitle failed: %w", err)
	}
	return nil
}

// UpdateConversationStats 更新会话的 token_used 和 role_count。
func UpdateConversationStats(convID string, tokenUsed, roleCount int) error {
	_, err := DB.Exec(`UPDATE t_conversation SET token_used = ?, role_count = ?, updated_time = ? WHERE conv_id = ?`,
		tokenUsed, roleCount, time.Now().Format("2006-01-02 15:04:05"), convID)
	if err != nil {
		return fmt.Errorf("UpdateConversationStats failed: %w", err)
	}
	return nil
}

// DeleteConversation 删除会话及其全部消息（事务内执行）。
func DeleteConversation(convID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("DeleteConversation: begin tx failed: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("[conversation] rollback failed: %v", rbErr)
			}
		}
	}()

	_, err = tx.ExecContext(ctx, `DELETE FROM t_conversation_msg WHERE conv_id = ?`, convID)
	if err != nil {
		return fmt.Errorf("DeleteConversation: delete msgs failed: %w", err)
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM t_conversation WHERE conv_id = ?`, convID)
	if err != nil {
		return fmt.Errorf("DeleteConversation: delete conv failed: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("DeleteConversation: commit failed: %w", err)
	}
	return nil
}

// ---- 消息 CRUD ----

// AppendMessage 追加一条会话消息。
func AppendMessage(msg *TConversationMsg) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	if msg.CreatedTime == "" {
		msg.CreatedTime = now
	}
	if msg.ToolCalls == "" {
		msg.ToolCalls = "[]"
	}
	if msg.EvidenceRefs == "" {
		msg.EvidenceRefs = "[]"
	}

	res, err := DB.Exec(`INSERT INTO t_conversation_msg
		(conv_id, turn, role, content, tool_calls, evidence_refs, token_count, created_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ConvId, msg.Turn, msg.Role, msg.Content,
		msg.ToolCalls, msg.EvidenceRefs, msg.TokenCount, msg.CreatedTime)
	if err != nil {
		return fmt.Errorf("AppendMessage failed: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("AppendMessage: get last insert id failed: %w", err)
	}
	msg.Id = id
	return nil
}

// GetMessagesByConvID 获取会话的全部消息（按 turn 升序）。
func GetMessagesByConvID(convID string) ([]TConversationMsg, error) {
	var msgs []TConversationMsg
	if err := DB.Select(&msgs,
		`SELECT id, conv_id, turn, role, content, tool_calls, evidence_refs, token_count, created_time
		 FROM t_conversation_msg WHERE conv_id = ? ORDER BY turn ASC`, convID); err != nil {
		return nil, fmt.Errorf("GetMessagesByConvID failed: %w", err)
	}
	return msgs, nil
}

// CountMessagesByConvID 统计会话消息数。
func CountMessagesByConvID(convID string) (int, error) {
	var count int
	if err := DB.Get(&count, `SELECT COUNT(*) FROM t_conversation_msg WHERE conv_id = ?`, convID); err != nil {
		return 0, fmt.Errorf("CountMessagesByConvID failed: %w", err)
	}
	return count, nil
}

// GetNextTurn 获取会话的下一个 turn 编号。
func GetNextTurn(convID string) (int, error) {
	var maxTurn int
	if err := DB.Get(&maxTurn, `SELECT COALESCE(MAX(turn), 0) FROM t_conversation_msg WHERE conv_id = ?`, convID); err != nil {
		return 0, fmt.Errorf("GetNextTurn failed: %w", err)
	}
	return maxTurn + 1, nil
}
