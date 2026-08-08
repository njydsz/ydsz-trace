import http from './http'

export function queryItems(page = 1, limit = 10) {
  return http.get('/item/queryPage', { params: { page, limit } })
}

export function queryAllItems() {
  return http.get('/item/queryAll')
}

export function getItem(id) {
  return http.get('/item/query', { params: { id } })
}

export function addItem(payload) {
  return http.post('/item/add', payload)
}

export function updateItem(payload) {
  return http.post('/item/update', payload)
}

export function deleteItem(id) {
  return http.get('/item/delete', { params: { id } })
}

export function changeItemStatus(id) {
  return http.get('/item/changeStatus', { params: { id } })
}
