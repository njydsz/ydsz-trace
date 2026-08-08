// 认证状态管理：基于 reactive + localStorage 的轻量 store。
//
// loggedIn 基于 username 是否存在判断（服务端会话过期由 http.js 拦截器跳转登录页）。
import { reactive } from 'vue'

// STORAGE_KEY localStorage 中持久化用户名的 key。
const STORAGE_KEY = 'ydsz_username'

// state 响应式认证状态。
const state = reactive({
  username: localStorage.getItem(STORAGE_KEY) || '',
})

// setUser 设置当前用户并同步写入 localStorage。
function setUser(username) {
  state.username = username || ''
  if (state.username) {
    localStorage.setItem(STORAGE_KEY, state.username)
  } else {
    localStorage.removeItem(STORAGE_KEY)
  }
}

// clear 清除当前用户与 localStorage。
function clear() {
  setUser('')
}

// useAuth 暴露认证状态与方法（组合式 API）。
export function useAuth() {
  return {
    state,
    setUser,
    clear,
    get loggedIn() {
      return !!state.username
    },
  }
}
