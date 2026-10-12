// 认证相关 API：登录、登出、健康检查
import http from './http'

// login 调用后端 /admin/login，返回会话 Cookie。
export function login(username, password) {
  return http.post('/admin/login', { username, password })
}

// logout 调用后端 /admin/exit 清除会话。
export function logout() {
  return http.get('/admin/exit')
}

export function health() {
  return http.get('/health')
}

// changePassword 调用后端 /admin/users/change-password 自助修改密码。
export function changePassword(oldPassword, newPassword) {
  return http.post('/admin/users/change-password', { oldPassword, newPassword })
}
