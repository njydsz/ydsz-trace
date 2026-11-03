import http from './http'

export function listRules(params) {
  return http.get('/logs/alerts/rules', { params: params || {} })
}

export function getRule(id) {
  return http.get(`/logs/alerts/rules/${id}`)
}

export function addRule(payload) {
  return http.post('/logs/alerts/rules', payload)
}

export function updateRule(id, payload) {
  return http.put(`/logs/alerts/rules/${id}`, payload)
}

export function deleteRule(id) {
  return http.delete(`/logs/alerts/rules/${id}`)
}

export function toggleRule(id, enabled) {
  return http.post('/logs/alerts/rules/toggle', { id, enabled })
}

export function testFireRule(id) {
  return http.post('/logs/alerts/rules/test', { id })
}

export function listEvents(params) {
  return http.get('/logs/alerts/events', { params: params || {} })
}

export function deleteEvent(id) {
  return http.delete(`/logs/alerts/events/${id}`)
}

export function getQuota() {
  return http.get('/logs/alerts/quota')
}

// ============ V2.2 异常反馈闭环 / 静默窗口 / 异常时间线 ============

export function getAnomalyTimeline(params) {
  return http.get('/logs/anomalies/timeline', { params: params || {} })
}

export function getFeedbackStats() {
  return http.get('/logs/anomalies/feedback/stats')
}

export function markAnomalyFeedback(payload) {
  return http.post('/logs/anomalies/feedback', payload)
}

export function listMaintenanceWindows() {
  return http.get('/logs/maintenance-windows')
}

export function saveMaintenanceWindows(windows) {
  return http.post('/logs/maintenance-windows', { windows: windows || [] })
}

export function syncMaintenanceWindows() {
  return http.post('/logs/maintenance-windows/sync')
}

export function listChannels() {
  return http.get('/logs/alerts/channels')
}

export function testChannel(payload) {
  return http.post('/logs/alerts/channels/test', payload)
}
