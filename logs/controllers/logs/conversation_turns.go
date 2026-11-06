// Package logs 子文件：会话轮次加载（V2.3-05）。
//
// 供 NL2DSL 指代消解读取历史轮次，实现多轮上下文补全。
//
// 数据来源：t_conversation_msg 表。
// 只取「用户输入」与「解析结果」，不取助手回复——助手回复是结论而非检索条件，
// 把它当上下文会引入无关关键词。
package logs

import (
	models "ydsz-trace/logs/models"
)

// loadConversationTurns 读取指定会话最近的历史轮次。
//
// 返回按时间正序的轮次列表（最新在最后），最多 maxContextTurns 条。
// 无记录或 DB 未初始化时返回 nil。
func loadConversationTurns(convID string) []ConversationTurn {
	if convID == "" || models.DB == nil {
		return nil
	}

	msgs, err := models.GetMessagesByConvID(convID)
	if err != nil || len(msgs) == 0 {
		return nil
	}

	turns := make([]ConversationTurn, 0, len(msgs))
	for _, m := range msgs {
		// 只取用户角色的消息作为检索条件来源
		if m.Role != "user" {
			continue
		}
		if m.Content == "" {
			continue
		}
		turns = append(turns, ConversationTurn{
			Query:  m.Content,
			Parsed: nl2dslBuildResponse(m.Content),
		})
	}

	return trimContextTurns(turns)
}
