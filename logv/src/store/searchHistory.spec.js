// 搜索历史与收藏 store 单元测试
//
// 覆盖场景：
//   - 历史记录去重与 LRU 淘汰（最多 20 条）
//   - 收藏添加/移除/判定
//   - localStorage 持久化与异常降级
//   - load 从 localStorage 恢复
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useSearchHistory } from './searchHistory'

describe('useSearchHistory store', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('初始状态：历史与收藏为空', () => {
    const store = useSearchHistory()
    expect(store.history).toEqual([])
    expect(store.favorites).toEqual([])
  })

  it('add：添加到历史并持久化', () => {
    const store = useSearchHistory()
    store.add({ query: 'timeout', params: { client: 1, item: 2 }, hitCount: 5 })
    expect(store.history).toHaveLength(1)
    expect(store.history[0].query).toBe('timeout')
    expect(store.history[0].hitCount).toBe(5)
    // 持久化验证
    const persisted = JSON.parse(localStorage.getItem('ydsz-search-history'))
    expect(persisted[0].query).toBe('timeout')
  })

  it('add：重复查询去重（移到最前）', () => {
    const store = useSearchHistory()
    store.add({ query: 'error', params: { item: 1 } })
    store.add({ query: 'timeout', params: { item: 1 } })
    store.add({ query: 'error', params: { item: 1 } }) // 重复
    expect(store.history).toHaveLength(2)
    expect(store.history[0].query).toBe('error') // 最新在前
  })

  it('add：超过 20 条自动淘汰最旧', () => {
    const store = useSearchHistory()
    for (let i = 0; i < 25; i++) {
      store.add({ query: `q${i}`, params: { i } })
    }
    expect(store.history).toHaveLength(20)
    expect(store.history[0].query).toBe('q24') // 最新
    expect(store.history[19].query).toBe('q5') // 第 6 条被保留
  })

  it('add：空查询忽略', () => {
    const store = useSearchHistory()
    store.add(null)
    store.add({ query: '' })
    expect(store.history).toHaveLength(0)
  })

  it('load：从 localStorage 恢复', () => {
    localStorage.setItem('ydsz-search-history', JSON.stringify([
      { query: 'prev-query', timestamp: 123, hitCount: 10, params: {} },
    ]))
    localStorage.setItem('ydsz-search-favorites', JSON.stringify([
      { query: 'fav-query', params: { item: 1 } },
    ]))
    const store = useSearchHistory()
    expect(store.history).toHaveLength(1)
    expect(store.history[0].query).toBe('prev-query')
    expect(store.favorites).toHaveLength(1)
    expect(store.favorites[0].query).toBe('fav-query')
  })

  it('favorite 添加/移除（使用 toggleFavorite）', () => {
    const store = useSearchHistory()
    const added = store.toggleFavorite({ query: 'critical-error', params: { item: 1 } })
    expect(added).toBe(true)
    expect(store.favorites).toHaveLength(1)
    expect(store.isFavorite('critical-error', 1)).toBe(true)
    const removed = store.toggleFavorite({ query: 'critical-error', params: { item: 1 } })
    expect(removed).toBe(false)
    expect(store.favorites).toHaveLength(0)
    expect(store.isFavorite('critical-error', 1)).toBe(false)
  })

  it('clearHistory：清空历史', () => {
    const store = useSearchHistory()
    store.add({ query: 'a' })
    store.add({ query: 'b' })
    store.clearHistory()
    expect(store.history).toHaveLength(0)
    expect(localStorage.getItem('ydsz-search-history')).toBe('[]')
  })

  it('localStorage 异常：load 失败回退为空', () => {
    localStorage.setItem('ydsz-search-history', 'not-valid-json')
    const store = useSearchHistory()
    expect(store.history).toEqual([])
  })
})
