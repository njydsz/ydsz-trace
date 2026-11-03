// 认证状态管理（Pinia store，保留 useAuth() 兼容接口以零成本接入现有调用方）。
//
// loggedIn 基于 username 是否存在判断（服务端会话过期由 http.js 拦截器清空并跳转登录页）。
// 底层使用 defineStore 以便后续扩展持久化 / 多 account 场景；
// useAuth() 暴露的 { state, setUser, clear, loggedIn } 形态与旧 reactive 版完全一致，
// layout/login/router/http 均无需改动。
import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

// STORAGE_KEY localStorage 中持久化用户名的 key。
const STORAGE_KEY = 'ydsz_username'
// ROLE_KEY localStorage 中持久化用户角色的 key。
const ROLE_KEY = 'ydsz_role'

// useAuthStorage — Pinia 组合式 store。
export const useAuthStorage = defineStore('auth', () => {
  const username = ref(localStorage.getItem(STORAGE_KEY) || '')
  const role = ref(localStorage.getItem(ROLE_KEY) || 'viewer')

  // setUser 设置当前用户并同步写入 localStorage。
  function setUser(name) {
    username.value = name || ''
    if (username.value) {
      localStorage.setItem(STORAGE_KEY, username.value)
    } else {
      localStorage.removeItem(STORAGE_KEY)
    }
  }

  // setRole 设置用户角色并同步写入 localStorage。
  function setRole(r) {
    role.value = r || 'viewer'
    localStorage.setItem(ROLE_KEY, role.value)
  }

  // clear 清除当前用户与 localStorage。
  function clear() {
    setUser('')
    role.value = 'viewer'
    localStorage.removeItem(ROLE_KEY)
  }

  // loggedIn — 基于 username 是否存在判断。
  const loggedIn = computed(() => !!username.value)

  return { username, role, setUser, setRole, clear, loggedIn }
})

// useAuth — 兼容旧接口：返回 { state, setUser, clear, loggedIn } 形态。
// state 即 Pinia store 实例，模板中 auth.state.username 可直接访问响应式值。
export function useAuth() {
  const store = useAuthStorage()
  return {
    get state() {
      return store
    },
    setUser(name) {
      return store.setUser(name)
    },
    setRole(role) {
      return store.setRole(role)
    },
    clear() {
      return store.clear()
    },
    get loggedIn() {
      return store.loggedIn
    },
    get role() {
      return store.role
    },
  }
}
