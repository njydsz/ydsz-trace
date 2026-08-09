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
