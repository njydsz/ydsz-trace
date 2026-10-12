// web/src/store/searchHistory.spec.js — 搜索历史 store 测试基线
//
// 覆盖：初始化从 localStorage 读取、add 追加记录（最多 20 条）、
// toggleFavorite 收藏切换、localStorage 不可用时降级。
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

describe('useSearchHistory', () => {
  let useSearchHistory

  beforeEach(async () => {
    localStorage.clear()
    setActivePinia(createPinia())
    vi.resetModules()
    useSearchHistory = (await import('./searchHistory')).useSearchHistory
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  describe('初始化从 localStorage 读取', () => {
    it('localStorage 为空时 history 与 favorites 初始化为空数组', () => {
      const store = useSearchHistory()
      expect(store.history).toEqual([])
      expect(store.favorites).toEqual([])
    })

    it('localStorage 有历史记录时初始化加载', () => {
      localStorage.setItem('ydsz-search-history', JSON.stringify([
        { query: 'ERROR', timestamp: 1000, hitCount: 5, params: {} },
        { query: 'timeout', timestamp: 2000, hitCount: 3, params: {} },
      ]))
      const store = useSearchHistory()
      expect(store.history.length).toBe(2)
      expect(store.history[0].query).toBe('ERROR')
    })

    it('localStorage 有收藏数据时初始化加载', () => {
      localStorage.setItem('ydsz-search-favorites', JSON.stringify([
        { query: 'ERROR', name: 'ERROR 查询', timestamp: 1000, params: { item: 1 } },
      ]))
      const store = useSearchHistory()
      expect(store.favorites.length).toBe(1)
      expect(store.favorites[0].query).toBe('ERROR')
    })

    it('localStorage 数据损坏时降级为空数组', () => {
      localStorage.setItem('ydsz-search-history', 'not-valid-json')
      const store = useSearchHistory()
      expect(store.history).toEqual([])
    })
  })

  describe('add 追加记录（最多 20 条）', () => {
    it('add 单条记录追加到列表头部', () => {
      const store = useSearchHistory()
      store.add({ query: 'test', timestamp: 1000 })
      expect(store.history[0].query).toBe('test')
    })

    it('add 重复 query + params 先去重再追加', () => {
      const store = useSearchHistory()
      store.add({ query: 'dup', timestamp: 1000, params: { item: 1 } })
      store.add({ query: 'dup', timestamp: 2000, params: { item: 1 } })
      // 相同 query+params 去重，只保留最新一条
      const count = store.history.filter(h => h.query === 'dup').length
      expect(count).toBe(1)
      expect(store.history[0].timestamp).toBe(2000)
    })

    it('add 超过 20 条后截断到 20 条', () => {
      const store = useSearchHistory()
      for (let i = 0; i < 25; i++) {
        store.add({ query: `q${i}`, timestamp: 1000 + i, params: {} })
      }
      expect(store.history.length).toBe(20)
      // 最新的排在最前
      expect(store.history[0].query).toBe('q24')
      expect(store.history[19].query).toBe('q5')
    })

    it('add 无 query 的记录不追加', () => {
      const store = useSearchHistory()
      store.add({ query: '', timestamp: 1000 })
      expect(store.history.length).toBe(0)
    })

    it('add 后同步到 localStorage', () => {
      const store = useSearchHistory()
      store.add({ query: 'persist', timestamp: 1234 })
      const raw = localStorage.getItem('ydsz-search-history')
      const parsed = JSON.parse(raw)
      expect(parsed[0].query).toBe('persist')
    })
  })

  describe('toggleFavorite 收藏切换', () => {
    it('toggleFavorite 添加收藏', () => {
      const store = useSearchHistory()
      const result = store.toggleFavorite({ query: 'mySearch', params: { item: 1 } })
      expect(result).toBe(true)
      expect(store.favorites.length).toBe(1)
      expect(store.favorites[0].query).toBe('mySearch')
    })

    it('toggleFavorite 再次调用取消收藏', () => {
      const store = useSearchHistory()
      store.toggleFavorite({ query: 'mySearch', params: { item: 1 } })
      const result = store.toggleFavorite({ query: 'mySearch', params: { item: 1 } })
      expect(result).toBe(false)
      expect(store.favorites.length).toBe(0)
    })

    it('isFavorite 判断收藏状态', () => {
      const store = useSearchHistory()
      store.toggleFavorite({ query: 'fav', params: { item: 2 } })
      expect(store.isFavorite('fav', 2)).toBe(true)
      expect(store.isFavorite('notfound', 2)).toBe(false)
    })
  })

  describe('clearHistory 与 remove', () => {
    it('clearHistory 清空所有历史记录', () => {
      const store = useSearchHistory()
      store.add({ query: 'a', timestamp: 1 })
      store.add({ query: 'b', timestamp: 2 })
      store.clearHistory()
      expect(store.history).toEqual([])
    })

    it('remove 按 query+timestamp 移除指定记录', () => {
      const store = useSearchHistory()
      store.add({ query: 'keep', timestamp: 100 })
      store.add({ query: 'remove', timestamp: 200 })
      store.remove('remove', 200)
      expect(store.history.length).toBe(1)
      expect(store.history[0].query).toBe('keep')
    })
  })

  describe('localStorage 不可用时降级', () => {
    it('localStorage.setItem 异常时静默忽略不报错', () => {
      vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new Error('QuotaExceededError')
      })
      const store = useSearchHistory()
      // add 应正常执行（safeSet 捕获异常），history 应更新
      expect(() => {
        store.add({ query: 'safe', timestamp: 1 })
      }).not.toThrow()
      expect(store.history.length).toBe(1)
    })

    it('localStorage.getItem 异常时初始化为空数组', () => {
      vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
        throw new Error('localStorage disabled')
      })
      const store = useSearchHistory()
      expect(store.history).toEqual([])
      expect(store.favorites).toEqual([])
    })
  })
})
