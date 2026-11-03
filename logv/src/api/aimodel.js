// 模型能力配置 API—与后端 logs/controllers/admin/aimodel.go 对齐。
// AI 状态面板 API（L0-L3 四层智能漏斗运行状态）。
import http from './http'

// 模型能力配置 CRUD ============

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

// selfCheckModelCapability 连通性自检：探测指定模型是否可达并返回延迟。
// @param {number} id - 模型能力配置 ID
export function selfCheckModelCapability(id) {
  return http.post('/admin/model-capability/selfcheck', { id })
}

// AI 状态面板 ============

// fetchAIStatus 获取 AI 四层运行状态（L0-L3 智能漏斗）。
export function fetchAIStatus() {
  return http.get('/admin/ai/status')
}
