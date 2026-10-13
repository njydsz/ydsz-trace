// 全局 axios 实例 + 响应拦截器 + CSRF 请求头注入。
//
// 默认 120s 超时（适配日志查询场景）。
// 响应拦截器职责：仅处理 HTTP 层错误（401/403/5xx）；
// 业务状态码由调用方在 store/composables 层统一判断，不在此处 reject。
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

// 是否为 JSON 响应（含 code 字段需要统一解包）
function isJsonResponse(response) {
  const ct = response.headers?.['content-type'] || ''
  return ct.includes('application/json')
}

/**
 * 判断业务状态码是否成功。
 * PRD V2.1 FR-1405：统一响应格式 code 为数字类型，0 = 成功。
 * 保留字符串兼容以应对过渡期混合部署。
 */
function isBusinessSuccess(code) {
  return code === 0 || code === '0'
}

http.interceptors.response.use(
  (response) => {
    // P1-5 修复：拦截器仅处理 HTTP 层错误，业务状态码由上层判断。
    // 对非成功的业务码给出 Toast 提示但不 reject，保持调用链可控。
    if (isJsonResponse(response) && response.data && 'code' in response.data) {
      const payload = response.data
      if (!isBusinessSuccess(payload.code)) {
        const msg = payload.message || payload.msg || '请求失败'
        ElMessage.warning(msg)
      }
    }
    return response
  },
  (error) => {
    const status = error.response?.status
    if (status === 401) {
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
    } else if (error.code === 'ECONNABORTED' || error.code === 'ETIMEDOUT') {
      ElMessage.error('请求超时，请稍后重试')
    }
    return Promise.reject(error)
  }
)

export default http
