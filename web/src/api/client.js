import http from './http'

export function queryClients(page = 1, limit = 10) {
  return http.get('/client/queryPage', { params: { page, limit } })
}

export function queryAllClients() {
  return http.get('/client/queryAll')
}

export function getClient(id) {
  return http.get('/client/query', { params: { id } })
}

export function addClient(payload) {
  return http.post('/client/add', payload)
}

export function updateClient(payload) {
  return http.post('/client/update', payload)
}

export function deleteClient(id) {
  return http.get('/client/delete', { params: { id } })
}

export function changeClientStatus(id) {
  return http.get('/client/changeStatus', { params: { id } })
}
