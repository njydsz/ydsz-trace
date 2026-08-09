// 全局 axios 实例 + 响应拦截器 + CSRF 请求头注入
//
// 默认 120s 超时（适配日志查询场景）；401 自动清空登录态跳登录页。
// 所有非 GET 请求附加 X-Requested-With 头，配合服务端 CSRF 中间件校验。
import axios from 'axios'
import { ElMessage } from 'element-plus'
import { useAuth } from '@/store/auth'

// 前后端同域：生产由 Go 服务端托管，开发由 Vite 代理转发。
const http = axios.create({
  baseURL: '',
  timeout: 120000,
  withCredentials: true,
  headers: { 'Content-Type': 'application/json' },
})

// 请求拦截：为非安全方法附加 CSRF 防护头
http.interceptors.request.use((config) => {
  const method = (config.method || 'get').toUpperCase()
  if (method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
    config.headers['X-Requested-With'] = 'XMLHttpRequest'
  }
  return config
})

http.interceptors.response.use(
  (response) => response,
  (error) => {
    const status = error.response?.status
    if (status === 401) {
      // 会话失效：清空登录态并跳回登录页
      const auth = useAuth()
      auth.clear()
      if (window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
    } else if (status === 403) {
      const msg = error.response?.data?.msg || '没有权限执行该操作'
      ElMessage.error(msg.includes('CSRF') ? '安全校验失败：请通过控制台发起操作' : msg)
    } else if (status && status >= 500) {
      ElMessage.error('服务端异常，请稍后重试')
    }
    return Promise.reject(error)
  }
)

export default http
