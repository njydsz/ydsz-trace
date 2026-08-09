// Auth store 单元测试 — 覆盖 useState / clear / loggedIn / localStorage 同步
//
// 测试环境：jsdom（vitest.config.js 已配置），Pinia createPinia() 每个用例前重置。
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore, useAuth } from './auth'

// 辅助：创建局部 localStorage 隔离（jsdom 默认会共享）
function cleanLocalStorage() {
  localStorage.clear()
}

describe('useAuthStore (Pinia defineStore)', () => {
  beforeEach(() => {
    cleanLocalStorage()
    setActivePinia(createPinia())
  })

  afterEach(() => {
    cleanLocalStorage()
    vi.restoreAllMocks()
  })

  it('初始状态：localStorage 无记录时 username 为空字符串', () => {
    const store = useAuthStore()
    expect(store.username).toBe('')
    expect(store.loggedIn).toBe(false)
  })

  it('初始状态：localStorage 有记录时从本地恢复 username', () => {
    localStorage.setItem('ydsz_username', 'alice')
    const store = useAuthStore()
    expect(store.username).toBe('alice')
    expect(store.loggedIn).toBe(true)
  })

  it('setUser 设置用户名并写入 localStorage', () => {
    const store = useAuthStore()
    store.setUser('bob')
    expect(store.username).toBe('bob')
    expect(store.loggedIn).toBe(true)
    expect(localStorage.getItem('ydsz_username')).toBe('bob')
  })

  it('setUser(""): 等价于 clear，清除 localStorage', () => {
    const store = useAuthStore()
    store.setUser('bob')
    store.setUser('')
    expect(store.username).toBe('')
    expect(store.loggedIn).toBe(false)
    expect(localStorage.getItem('ydsz_username')).toBe(null)
  })

  it('setUser(undefined) / null: 覆盖为空', () => {
    const store = useAuthStore()
    store.setUser('bob')
    store.setUser(undefined)
    expect(store.username).toBe('')
    expect(localStorage.getItem('ydsz_username')).toBe(null)
  })

  it('clear 后 loggedIn 变为 false', () => {
    const store = useAuthStore()
    store.setUser('alice')
    expect(store.loggedIn).toBe(true)
    store.clear()
    expect(store.loggedIn).toBe(false)
  })

  it('多个实例相互独立（不同 pinia store id）', () => {
    const a = useAuthStore()
    a.setUser('aaa')
    const b = useAuthStore()
    // 同一 pinia 实例下，useAuthStore() 返回同一个 store
    expect(b.username).toBe('aaa')
    expect(b).toBe(a)
  })

  it('localStorage.setItem 外部修改不反向同步到 store（单向持久化）', () => {
    const store = useAuthStore()
    localStorage.setItem('ydsz_username', 'injected')
    // store 初始化后不会主动监听 localStorage 外部变更
    expect(store.username).toBe('')
  })
})

describe('useAuth (兼容接口)', () => {
  beforeEach(() => {
    cleanLocalStorage()
    setActivePinia(createPinia())
  })

  afterEach(() => cleanLocalStorage())

  it('返回兼容对象结构 { state, setUser, clear, loggedIn }', () => {
    const auth = useAuth()
    expect(auth).toHaveProperty('state')
    expect(auth).toHaveProperty('setUser')
    expect(auth).toHaveProperty('clear')
    expect(auth).toHaveProperty('loggedIn')
  })

  it('auth.state.username 与 store.username 同值响应', () => {
    const auth = useAuth()
    auth.setUser('charlie')
    expect(auth.state.username).toBe('charlie')
    expect(auth.loggedIn).toBe(true)
  })

  it('派生 getters 跨调用反应最新值', () => {
    const auth = useAuth()
    expect(auth.loggedIn).toBe(false)
    auth.setUser('dave')
    expect(auth.loggedIn).toBe(true)
    auth.clear()
    expect(auth.loggedIn).toBe(false)
  })

  it('clear 后 localStorage 被清空', () => {
    const auth = useAuth()
    auth.setUser('eve')
    expect(localStorage.getItem('ydsz_username')).toBe('eve')
    auth.clear()
    expect(localStorage.getItem('ydsz_username')).toBe(null)
  })

  it('未传用户名时 setUser 安全兜底为空', () => {
    const auth = useAuth()
    auth.setUser('someone')
    auth.setUser()
    expect(auth.state.username).toBe('')
    expect(auth.loggedIn).toBe(false)
  })
})

describe('Pinia 多实例隔离（每个 pinia 实例独立 state）', () => {
  beforeEach(() => cleanLocalStorage())
  afterEach(() => cleanLocalStorage())

  it('两次 createPinia 创建独立的 store 状态，互不污染（需先隔离 localStorage）', () => {
    // Pinia design: defineStore 注册 store 工厂到全局 symbol，但每次 createPinia() 生成新的 state ref，
    // 所以每个 pinia 实例持有独立的应用状态视图。注意：当前 setup store 从 localStorage 初始化，
    // 而 jsdom 下 localStorage 跨实例共享，因此这里先清空以隔离测试环境。
    setActivePinia(createPinia())
    const a = useAuthStore()
    a.setUser('u-a')

    // 清理 localStorage（模拟切换浏览器标签/无痕窗口）
    cleanLocalStorage()
    setActivePinia(createPinia())
    const b = useAuthStore()

    // b 是新 pinia 实例的 store，无 localStorage 残留应初始为空
    expect(b.username).toBe('')
    expect(b.loggedIn).toBe(false)

    // a 仍然持有旧 pinia 实例的 state
    expect(a.username).toBe('u-a')
    expect(a).not.toBe(b)
  })

  it('不同 pinia 实例的 store state 互不影响', () => {
    setActivePinia(createPinia())
    const a = useAuthStore()
    a.setUser('first')

    setActivePinia(createPinia())
    const b = useAuthStore()
    b.setUser('second')

    // a 与 b 指向不同 pinia 实例中的独立 store
    expect(a.username).toBe('first')
    expect(b.username).toBe('second')
  })
})
