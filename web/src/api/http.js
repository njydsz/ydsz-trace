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
      ElMessage.error('没有权限执行该操作')
    } else if (status && status >= 500) {
      ElMessage.error('服务端异常，请稍后重试')
    }
    return Promise.reject(error)
  }
)

export default http
