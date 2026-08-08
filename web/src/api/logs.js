import http from './http'

// 所有客户端列表（日志检索页使用）
export function queryLogClients() {
  return http.get('/logs/queryClients')
}

// 根据客户端 ID 查询其下的日志项
export function queryItemsByClient(clientId) {
  return http.get('/logs/queryItems', { params: { client_id: clientId } })
}

// 发起日志检索：返回 zip 文件流（blob）
export function queryLogs(payload) {
  return http.post('/logs/query', payload, { responseType: 'blob' })
}
