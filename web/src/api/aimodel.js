import http from './http'

// 模型能力配置 API—与后端 logs/controllers/admin/aimodel.go 对齐。

// listModelCapabilities 查询模型能力配置列表。
// params: { pageNo, pageSize, keyword, capability }
export function listModelCapabilities(params = {}) {
  return http.get('/admin/model-capability/list', { params })
}

// getModelCapability 获取单个模型能力配置详情。
export function getModelCapability(id) {
  return http.get(`/admin/model-capability/${id}`)
}

// addModelCapability 新增模型能力配置。
export function addModelCapability(data) {
  return http.post('/admin/model-capability', data)
}

// updateModelCapability 更新模型能力配置。
export function updateModelCapability(data) {
  return http.put('/admin/model-capability', data)
}

// deleteModelCapability 删除模型能力配置。
export function deleteModelCapability(id) {
  return http.post('/admin/model-capability/delete', { id })
}

// toggleModelCapability 启用/禁用模型能力配置。
export function toggleModelCapability(id, enabled) {
  return http.post('/admin/model-capability/toggle', { id, enabled })
}
