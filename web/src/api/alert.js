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
