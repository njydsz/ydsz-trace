import http from './http'

// queryLogClients 查询所有客户端（日志检索页下拉框使用）。
export function queryLogClients() {
  return http.get('/logs/queryClients')
}

// queryItemsByClient 根据 client_id 查询其下的日志项。
export function queryItemsByClient(clientId) {
  return http.get('/logs/queryItems', { params: { client_id: clientId } })
}

// queryLogs 发起日志检索（返回 zip 文件流，responseType=blob）。
export function queryLogs(payload) {
  return http.post('/logs/query', payload, { responseType: 'blob' })
}

// searchLogs 发起在线分页检索（返回 JSON 行列表）。
export function searchLogs(payload) {
  return http.post('/logs/search', payload)
}

// traceLogs 按 traceId 跨节点聚合调用链片段。
export function traceLogs(payload) {
  return http.post('/logs/trace', payload)
}

// traceWaterfall 按 traceId 检索并返回瀑布视图数据（按时间排序的 spans）。
export function traceWaterfall(payload) {
  return http.post('/logs/trace/waterfall', payload)
}

// logContext 获取日志行上下文（命中前后 N 行）。
// @param {number} itemId - 日志项 ID
// @param {object} params - 查询参数 { clientId, refTime, refLine, before, after, date }
export function logContext(itemId, params = {}) {
  return http.get(`/logs/context/${itemId}`, { params })
}

// queryTaskProgress 轮询检索任务的实时进度（百分比 + 预估总数）。
// @param {string} taskNo - 任务编号
export function queryTaskProgress(taskNo) {
  return http.get(`/logs/tasks/${taskNo}/progress`)
}

// dashboardInsight 获取洞察看板数据（模板趋势/异常事件时间线/服务健康度）。
// @param {object} params - { timeRange, itemId }
export function dashboardInsight(params = {}) {
  return http.get('/logs/dashboard/insight', { params })
}

// searchSemantic 语义检索：基于向量相似度查找相似日志模板（FR-601）。
// @param {object} payload - { query, type, topK }
export function searchSemantic(payload) {
  return http.post('/logs/semantic/search', payload)
}

// searchTemplates 模板自动补全：按关键字搜索日志模板。
// @param {string} keyword - 搜索关键字
// @param {number} limit - 返回条数（默认 10）
export function searchTemplates(keyword, limit = 10) {
  return http.get('/logs/templates/search', { params: { q: keyword, limit } })
}

// ===== AI 诊断会话 API（FR-901）=====

// createConversation 创建新会话。
// @param {string} title - 会话标题
// @returns {Promise<object>} 响应含 convId
export function createConversation(title) {
  return http.post('/logs/conversation/create', { title })
}

// listConversations 查询当前用户的会话列表（分页）。
// @param {number} pageNo - 页码
// @param {number} pageSize - 每页条数
export function listConversations(pageNo = 1, pageSize = 20) {
  return http.get('/logs/conversation/list', { params: { pageNo, pageSize } })
}

// getConversationMessages 获取会话的全部消息。
// @param {string} convId - 会话 ID
export function getConversationMessages(convId) {
  return http.get(`/logs/conversation/${convId}/messages`)
}

// appendConversationMessage 追加一条消息到会话。
// @param {string} convId - 会话 ID
// @param {string} role - 角色 (user / assistant / system)
// @param {string} content - 消息内容
// @param {object} extra - 可选字段 { toolCalls, evidenceRefs, tokenCount }
export function appendConversationMessage(convId, role, content, extra = {}) {
  return http.post('/logs/conversation/append', {
    convId,
    role,
    content,
    ...extra,
  })
}

// deleteConversation 删除会话及其全部消息。
// @param {string} convId - 会话 ID
export function deleteConversation(convId) {
  return http.delete(`/logs/conversation/${convId}`)
}

// updateConversationTitle 更新会话标题。
// @param {string} convId - 会话 ID
// @param {string} title - 新标题
export function updateConversationTitle(convId, title) {
  return http.put(`/logs/conversation/${convId}/title`, { title })
}

// ===== 根因分析 Agent API（FR-901~908）=====

// runAgentReasoning 执行根因分析（ReAct 循环 + 三段式输出）。
// @param {string} query - 用户问题
// @param {string} convId - 关联会话 ID（可选）
// @returns {Promise<object>} 三段式结果 { hypotheses, evidence, conclusion, steps, toolCalls, degraded }
export function runAgentReasoning(query, convId = '') {
  return http.post('/logs/agent/reason', { query, convId })
}

// listAgentTools 列出 Agent 可用工具。
// @returns {Promise<object>} 工具列表 [{ name, description }]
export function listAgentTools() {
  return http.get('/logs/agent/tools')
}
