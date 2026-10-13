// 搜索历史与收藏 Pinia store
//
// localStorage key: 'ydsz-search-history' / 'ydsz-search-favorites'
// 历史最多 20 条，去重；收藏无上限
import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

const HISTORY_KEY = 'ydsz-search-history'
const FAVORITES_KEY = 'ydsz-search-favorites'
const MAX_HISTORY = 20

// safeLocalStorage 封装 localStorage 读写，不可用时降级为内存状态
function safeGet(key) {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function safeSet(key, value) {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* localStorage 不可用时忽略 */
  }
}

// useSearchHistory — 搜索历史与收藏 store。
export const useSearchHistory = defineStore('searchHistory', () => {
  const history = ref([])
  const favorites = ref([])

  const favoriteQueries = computed(() =>
    new Set(favorites.value.map(f => f.query + '|' + f.params?.item))
  )

  // load 从 localStorage 读取
  function load() {
    try {
      const rawHist = safeGet(HISTORY_KEY)
      history.value = rawHist ? JSON.parse(rawHist) : []
    } catch {
      history.value = []
    }
    try {
      const rawFav = safeGet(FAVORITES_KEY)
      favorites.value = rawFav ? JSON.parse(rawFav) : []
    } catch {
      favorites.value = []
    }
  }

  // saveHistory 写入历史到 localStorage
  function saveHistory() {
    safeSet(HISTORY_KEY, JSON.stringify(history.value))
  }

  // saveFavorites 写入收藏到 localStorage
  function saveFavorites() {
    safeSet(FAVORITES_KEY, JSON.stringify(favorites.value))
  }

  // add 添加到历史（去重，保留最新 20）
  function add(entry) {
    if (!entry || !entry.query) return
    const dupIdx = history.value.findIndex(
      h => h.query === entry.query && JSON.stringify(h.params) === JSON.stringify(entry.params)
    )
    if (dupIdx >= 0) {
      history.value.splice(dupIdx, 1)
    }
    history.value.unshift({
      query: entry.query,
      timestamp: entry.timestamp || Date.now(),
      hitCount: entry.hitCount ?? 0,
      params: entry.params || {},
    })
    if (history.value.length > MAX_HISTORY) {
      history.value = history.value.slice(0, MAX_HISTORY)
    }
    saveHistory()
  }

  // remove 从历史移除（按 query + timestamp 唯一定位）
  function remove(query, timestamp) {
    history.value = history.value.filter(h => !(h.query === query && h.timestamp === timestamp))
    saveHistory()
  }

  // toggleFavorite 切换收藏状态
  function toggleFavorite(entry) {
    const key = (entry.query || '') + '|' + (entry.params?.item || '')
    const idx = favorites.value.findIndex(
      f => (f.query || '') + '|' + (f.params?.item || '') === key
    )
    if (idx >= 0) {
      favorites.value.splice(idx, 1)
      saveFavorites()
      return false
    }
    favorites.value.unshift({
      query: entry.query || '',
      name: entry.name || entry.query || '',
      timestamp: entry.timestamp || Date.now(),
      params: entry.params || {},
    })
    saveFavorites()
    return true
  }

  // isFavorite 判断是否已收藏
  function isFavorite(query, itemId) {
    const key = (query || '') + '|' + (itemId || '')
    return favoriteQueries.value.has(key)
  }

  // clearHistory 清空历史
  function clearHistory() {
    history.value = []
    saveHistory()
  }

  // 初始化加载
  load()

  return {
    history,
    favorites,
    load,
    saveHistory,
    saveFavorites,
    add,
    remove,
    toggleFavorite,
    isFavorite,
    clearHistory,
  }
})
