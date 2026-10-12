// 主题状态管理（Pinia store）。
//
// 主题：'light' | 'dark' | 'system'
// 持久化到 localStorage，key = 'ydsz-theme-preference'
import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

const STORAGE_KEY = 'ydsz-theme-preference'

function readPref() {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* localStorage 不可用时忽略 */
  }
  return 'system'
}

function isSystemDark() {
  try {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches
  } catch {
    return false
  }
}

function applyToDOM(theme, systemDark) {
  const root = document.documentElement
  if (theme === 'dark' || (theme === 'system' && systemDark)) {
    root.setAttribute('data-theme', 'dark')
  } else {
    root.removeAttribute('data-theme')
  }
}

export const useThemeStore = defineStore('theme', () => {
  const preference = ref(readPref())
  const systemDark = ref(isSystemDark())
  const systemDarkMQ = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)')

  // resolvedTheme — 将偏好解析为实际生效的 'light' | 'dark'。
  const resolvedTheme = computed(() => {
    if (preference.value === 'dark') return 'dark'
    if (preference.value === 'system') return systemDark.value ? 'dark' : 'light'
    return 'light'
  })

  // setPreference 设置主题偏好并同步到 localStorage + DOM。
  function setPreference(next) {
    preference.value = next
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      /* localStorage 不可用时忽略 */
    }
    applyToDOM(next, systemDark.value)
  }

  // cyclePreference 循环切换 system → light → dark → system。
  function cyclePreference() {
    const order = { system: 'light', light: 'dark', dark: 'system' }
    setPreference(order[preference.value] || 'system')
  }

  // syncSystemDark 响应系统主题变更（仅在偏好为 system 时生效）。
  function onMQChange(e) {
    systemDark.value = !!e.matches
    applyToDOM(preference.value, systemDark.value)
  }

  // startWatching 监听系统主题变更；返回取消监听的函数。
  function startWatching() {
    if (!systemDarkMQ) return () => {}
    systemDarkMQ.addEventListener('change', onMQChange)
    return () => systemDarkMQ.removeEventListener('change', onMQChange)
  }

  // init 应用当前偏好到 DOM 并启动监听。
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
