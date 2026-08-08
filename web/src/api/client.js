// 客户端（logc agent）管理 API
import http from './http'

// queryClients 分页查询客户端列表。
export function queryClients(page = 1, limit = 10) {
  return http.get('/client/queryPage', { params: { page, limit } })
}

// queryAllClients 查询全部客户端（用于下拉框等）。
export function queryAllClients() {
  return http.get('/client/queryAll')
}

// getClient 按 id 查询单个客户端详情。
export function getClient(id) {
  return http.get('/client/query', { params: { id } })
}

// addClient 新增客户端。
export function addClient(payload) {
  return http.post('/client/add', payload)
}

// updateClient 更新客户端。
export function updateClient(payload) {
  return http.post('/client/update', payload)
}

// deleteClient 按 id 删除客户端。
export function deleteClient(id) {
  return http.get('/client/delete', { params: { id } })
}

// changeClientStatus 切换客户端启用/禁用。
export function changeClientStatus(id) {
  return http.get('/client/changeStatus', { params: { id } })
}
