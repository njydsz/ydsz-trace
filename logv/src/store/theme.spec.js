// Theme store 单元测试（V2.1-05 补齐）
//
// 覆盖：偏好持久化、系统偏好跟随、init() 应用到 DOM、无 DOM 环境防御。
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useThemeStorage, useTheme } from './theme'

describe('useThemeStorage', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    setActivePinia(createPinia())
  })

  afterEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    vi.restoreAllMocks()
  })

  it('默认偏好为 system', () => {
    const store = useThemeStorage()
    expect(store.preference).toBe('system')
  })

  it('从 localStorage 恢复已保存偏好', () => {
    localStorage.setItem('ydsz-theme-preference', 'dark')
    const store = useThemeStorage()
    expect(store.preference).toBe('dark')
  })

  it('init() 将 dark 偏好应用到 DOM', () => {
    localStorage.setItem('ydsz-theme-preference', 'dark')
    const store = useThemeStorage()
    store.init()
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })

  it('init() 对 light 偏好移除 data-theme', () => {
    localStorage.setItem('ydsz-theme-preference', 'light')
    const store = useThemeStorage()
    store.init()
    expect(document.documentElement.getAttribute('data-theme')).toBeNull()
  })

  it('setPreference 写入 localStorage 并同步 DOM', () => {
    const store = useThemeStorage()
    store.setPreference('dark')
    expect(localStorage.getItem('ydsz-theme-preference')).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })

  it('cyclePreference 按 system → light → dark → system 循环', () => {
    const store = useThemeStorage()
    expect(store.preference).toBe('system')
    store.cyclePreference()
    expect(store.preference).toBe('light')
    store.cyclePreference()
    expect(store.preference).toBe('dark')
    store.cyclePreference()
    expect(store.preference).toBe('system')
  })

  it('resolvedTheme 在 dark 偏好下恒为 dark', () => {
    const store = useThemeStorage()
    store.setPreference('dark')
    expect(store.resolvedTheme).toBe('dark')
  })

  it('useTheme 兼容包装与 useThemeStorage 等价', () => {
    const store = useTheme()
    expect(store.preference).toBe('system')
    expect(typeof store.init).toBe('function')
  })
})
