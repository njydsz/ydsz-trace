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
