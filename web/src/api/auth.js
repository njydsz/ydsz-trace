import http from './http'

export function login(username, password) {
  return http.post('/admin/login', { username, password })
}

export function logout() {
  return http.get('/admin/exit')
}

export function health() {
  return http.get('/health')
}
