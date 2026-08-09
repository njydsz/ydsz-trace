// 项目日志项（t_item）管理 API
import http from './http'

// queryItems 分页查询项目日志列表。
export function queryItems(page = 1, limit = 10) {
  return http.get('/item/queryPage', { params: { page, limit } })
}

// queryAllItems 查询全部项目日志（用于下拉框等）。
export function queryAllItems() {
  return http.get('/item/queryAll')
}

// getItem 按 id 查询单个项目日志详情。
export function getItem(id) {
  return http.get('/item/query', { params: { id } })
}

// addItem 新增项目日志。
export function addItem(payload) {
  return http.post('/item/add', payload)
}

// updateItem 更新项目日志。
export function updateItem(payload) {
  return http.post('/item/update', payload)
}

// deleteItem 按 id 删除项目日志（POST + JSON body，配合 CSRF 防护）。
export function deleteItem(id) {
  return http.post('/item/delete', { id })
}

// changeItemStatus 设置项目日志启用/禁用状态（POST + JSON body：{ id, status }）。
export function changeItemStatus(id, status) {
  return http.post('/item/changeStatus', { id, status })
}
