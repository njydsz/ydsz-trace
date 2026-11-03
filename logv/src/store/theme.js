// 主题状态管理（Pinia store）。
//
// 主题：'light' | 'dark' | 'system'
// 持久化到 localStorage，key = 'ydsz-theme-preference'
//
// 设计要点：
//   - SSR/测试环境下 window 可能不存在，所有 DOM/媒体查询访问均需防御
//   - 必须在应用启动时调用 init()，否则首屏不会应用已保存的偏好
import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

const STORAGE_KEY = 'ydsz-theme-preference'

// hasDOM 判断当前是否具备浏览器 DOM 环境。
function hasDOM() {
  return typeof window !== 'undefined' && typeof document !== 'undefined'
}

function readPref() {
  if (!hasDOM()) return 'system'
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* localStorage 不可用时忽略 */
  }
  return 'system'
}

function isSystemDark() {
  if (!hasDOM()) return false
  try {
    return !!(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches)
  } catch {
    return false
  }
}

function applyToDOM(theme, systemDark) {
  if (!hasDOM()) return
  const root = document.documentElement
  if (theme === 'dark' || (theme === 'system' && systemDark)) {
    root.setAttribute('data-theme', 'dark')
  } else {
    root.removeAttribute('data-theme')
  }
}

// useThemeStorage — 主题状态管理 Pinia store。
//
// 偏好：'light' | 'dark' | 'system'；持久化到 localStorage，key = 'ydsz-theme-preference'。
export const useThemeStorage = defineStore('theme', () => {
  const preference = ref(readPref())
  const systemDark = ref(isSystemDark())
  const systemDarkMQ = hasDOM()
    ? window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)')
    : null

  // resolvedTheme — 将偏好解析为实际生效的 'light' | 'dark'。
  const resolvedTheme = computed(() => {
    if (preference.value === 'dark') return 'dark'
    if (preference.value === 'system') return systemDark.value ? 'dark' : 'light'
    return 'light'
  })

  // setPreference 设置主题偏好并同步到 localStorage + DOM。
  function setPreference(next) {
    preference.value = next
    if (hasDOM()) {
      try {
        localStorage.setItem(STORAGE_KEY, next)
      } catch {
        /* localStorage 不可用时忽略 */
      }
    }
    applyToDOM(next, systemDark.value)
  }

  // cyclePreference 循环切换 system → light → dark → system。
  function cyclePreference() {
    const order = { system: 'light', light: 'dark', dark: 'system' }
    setPreference(order[preference.value] || 'system')
  }

  // onMQChange 响应系统主题变更（仅在偏好为 system 时生效）。
  function onMQChange(e) {
    systemDark.value = !!e.matches
    applyToDOM(preference.value, systemDark.value)
  }

  // startWatching 监听系统主题变更；返回取消监听的函数。
  function startWatching() {
    if (!systemDarkMQ || typeof systemDarkMQ.addEventListener !== 'function') {
      return () => {}
    }
    systemDarkMQ.addEventListener('change', onMQChange)
    return () => {
      if (systemDarkMQ && typeof systemDarkMQ.removeEventListener === 'function') {
        systemDarkMQ.removeEventListener('change', onMQChange)
      }
    }
  }

  // init 应用当前偏好到 DOM 并启动系统主题监听。
  //
  // 必须在应用挂载时调用一次（见 main.js），否则首屏不会应用已保存的偏好。
  function init() {
    applyToDOM(preference.value, systemDark.value)
    return startWatching()
  }

  return {
    preference,
    resolvedTheme,
    setPreference,
    cyclePreference,
    init,
  }
})

/**
 * useTheme 兼容包装函数，等价于 useThemeStorage。
 * 供旧代码迁移期间保持兼容。
 */
export function useTheme() {
  return useThemeStorage()
}
