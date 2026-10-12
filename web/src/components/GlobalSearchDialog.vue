<template>
  <el-dialog
    v-model="dialogVisible"
    :show-close="true"
    :close-on-click-modal="true"
    :close-on-press-escape="true"
    custom-class="global-search-dialog"
    width="640px"
    align-center
    @opened="onOpened"
    @closed="onClosed"
  >
    <div class="global-search">
      <!-- 搜索输入 -->
      <div class="flex items-center gap-3 px-2 pb-3 border-b border-gray-200 dark:border-gray-700">
        <el-icon class="text-lg text-gray-400 flex-shrink-0"><Search /></el-icon>
        <input
          ref="inputRef"
          v-model="searchInput"
          type="text"
          class="flex-1 bg-transparent outline-none text-base text-gray-800 dark:text-gray-200 placeholder-gray-400"
          placeholder="搜索日志项、客户端、历史查询..."
          @input="onInput"
          @keydown="onKeydown"
        />
        <kbd class="hidden sm:inline-block px-2 py-0.5 text-xs text-gray-400 border border-gray-300 dark:border-gray-600 rounded">ESC</kbd>
      </div>

      <!-- 搜索结果 -->
      <div class="mt-3 max-h-96 overflow-y-auto">
        <!-- 空状态 -->
        <div v-if="searchInput && !hasResults" class="text-center text-sm text-gray-400 py-8">
          未找到匹配结果
        </div>

        <!-- 搜索历史 -->
        <div v-if="!searchInput && (store.history.length > 0 || store.favorites.length > 0)">
          <div v-if="store.favorites.length > 0" class="mb-2">
            <div class="text-xs font-semibold text-gray-400 px-2 mb-1">收藏</div>
            <div
              v-for="(fav, idx) in store.favorites"
              :key="'fav-' + idx"
              class="result-item flex items-center gap-2 px-3 py-2 rounded-lg cursor-pointer"
              :class="{ 'bg-gray-100 dark:bg-gray-700': activeIndex === getGlobalIndex('favorite', idx) }"
              @mousedown.prevent="handleSelectFavorite(fav, idx)"
              @mouseenter="activeIndex = getGlobalIndex('favorite', idx)"
            >
              <el-icon class="text-yellow-500 flex-shrink-0"><Star /></el-icon>
              <span class="flex-1 text-sm text-gray-700 dark:text-gray-200 truncate">{{ fav.name || fav.query }}</span>
              <span class="text-xs text-gray-400">{{ fav.params?.itemName || '' }}</span>
            </div>
          </div>
          <div v-if="store.history.length > 0">
            <div class="text-xs font-semibold text-gray-400 px-2 mb-1">最近搜索</div>
            <div
              v-for="(h, idx) in store.history.slice(0, 5)"
              :key="'hist-' + idx"
              class="result-item flex items-center gap-2 px-3 py-2 rounded-lg cursor-pointer"
              :class="{ 'bg-gray-100 dark:bg-gray-700': activeIndex === getGlobalIndex('history', idx) }"
              @mousedown.prevent="handleSelectHistory(h, idx)"
              @mouseenter="activeIndex = getGlobalIndex('history', idx)"
            >
              <el-icon class="text-gray-400 flex-shrink-0"><Clock /></el-icon>
              <span class="flex-1 text-sm text-gray-700 dark:text-gray-200 truncate">{{ h.query }}</span>
              <span v-if="h.params?.itemName" class="text-xs text-gray-400">{{ h.params.itemName }}</span>
            </div>
          </div>
        </div>

        <!-- 日志项匹配 -->
        <div v-if="matchedItems.length > 0">
          <div class="text-xs font-semibold text-gray-400 px-2 mb-1">日志项</div>
          <div
            v-for="(item, idx) in matchedItems"
            :key="'item-' + item.id"
            class="result-item flex items-center gap-2 px-3 py-2 rounded-lg cursor-pointer"
            :class="{ 'bg-gray-100 dark:bg-gray-700': activeIndex === getGlobalIndex('item', idx) }"
            @mousedown.prevent="handleSelectItem(item, idx)"
            @mouseenter="activeIndex = getGlobalIndex('item', idx)"
          >
            <el-icon class="text-blue-500 flex-shrink-0"><Document /></el-icon>
            <div class="flex-1 min-w-0">
              <div class="text-sm text-gray-700 dark:text-gray-200 truncate">{{ item.itemName }}</div>
              <div v-if="item.path" class="text-xs text-gray-400 truncate">{{ item.path }}</div>
            </div>
          </div>
        </div>

        <!-- 客户端匹配 -->
        <div v-if="matchedClients.length > 0">
          <div class="text-xs font-semibold text-gray-400 px-2 mb-1">客户端</div>
          <div
            v-for="(client, idx) in matchedClients"
            :key="'client-' + client.id"
            class="result-item flex items-center gap-2 px-3 py-2 rounded-lg cursor-pointer"
            :class="{ 'bg-gray-100 dark:bg-gray-700': activeIndex === getGlobalIndex('client', idx) }"
            @mousedown.prevent="handleSelectClient(client, idx)"
            @mouseenter="activeIndex = getGlobalIndex('client', idx)"
          >
            <el-icon class="text-green-500 flex-shrink-0"><Cpu /></el-icon>
            <div class="flex-1 min-w-0">
              <div class="text-sm text-gray-700 dark:text-gray-200 truncate">{{ client.ip }}:{{ client.port }}</div>
              <div v-if="client.name" class="text-xs text-gray-400 truncate">{{ client.name }}</div>
            </div>
          </div>
        </div>

        <!-- 搜索历史匹配 -->
        <div v-if="matchedHistory.length > 0">
          <div class="text-xs font-semibold text-gray-400 px-2 mb-1">搜索历史</div>
          <div
            v-for="(h, idx) in matchedHistory"
            :key="'mh-' + idx"
            class="result-item flex items-center gap-2 px-3 py-2 rounded-lg cursor-pointer"
            :class="{ 'bg-gray-100 dark:bg-gray-700': activeIndex === getGlobalIndex('matchedHistory', idx) }"
            @mousedown.prevent="handleSelectMatchedHistory(h, idx)"
            @mouseenter="activeIndex = getGlobalIndex('matchedHistory', idx)"
          >
            <el-icon class="text-gray-400 flex-shrink-0"><Clock /></el-icon>
            <span class="flex-1 text-sm text-gray-700 dark:text-gray-200 truncate">{{ h.query }}</span>
            <span v-if="h.hitCount" class="text-xs text-gray-400">{{ h.hitCount }} 条</span>
          </div>
        </div>
      </div>

      <!-- 底部快捷键提示 -->
      <div class="flex items-center gap-4 mt-4 px-2 pt-3 border-t border-gray-100 dark:border-gray-700 text-xs text-gray-400">
        <span><kbd class="px-1 py-0.5 border border-gray-300 dark:border-gray-600 rounded mr-1">↑↓</kbd> 选择</span>
        <span><kbd class="px-1 py-0.5 border border-gray-300 dark:border-gray-600 rounded mr-1">↵</kbd> 确认</span>
        <span><kbd class="px-1 py-0.5 border border-gray-300 dark:border-gray-600 rounded mr-1">ESC</kbd> 关闭</span>
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
/**
 * GlobalSearchDialog — 全局搜索对话框（cmd/ctrl+k 唤起）
 * 等分三组：日志项、客户端、搜索历史；支持键盘快速导航。
 */
import { ref, computed, nextTick, defineExpose } from 'vue'
import { useRouter } from 'vue-router'
import { Search, Star, Clock, Document, Cpu } from '@element-plus/icons-vue'
import { useSearchHistory } from '@/store/searchHistory'
import { queryAllItems } from '@/api/item'
import { queryAllClients } from '@/api/client'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const router = useRouter()
const store = useSearchHistory()

const dialogVisible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
})
const searchInput = ref('')
const activeIndex = ref(-1)
const inputRef = ref(null)
const items = ref([])
const clients = ref([])

// 模糊匹配函数
function fuzzyMatch(text, query) {
  if (!query) return true
  if (!text) return false
  return String(text).toLowerCase().includes(query.toLowerCase())
}

// 日志项匹配结果
const matchedItems = computed(() => {
  if (!searchInput.value) return []
  return items.value.filter(it =>
    fuzzyMatch(it.itemName, searchInput.value) || fuzzyMatch(it.path, searchInput.value)
  ).slice(0, 8)
})

// 客户端匹配结果
const matchedClients = computed(() => {
  if (!searchInput.value) return []
  return clients.value.filter(c =>
    fuzzyMatch(c.ip, searchInput.value) || fuzzyMatch(c.name, searchInput.value) || fuzzyMatch(String(c.port), searchInput.value)
  ).slice(0, 8)
})

// 历史匹配结果
const matchedHistory = computed(() => {
  if (!searchInput.value) return []
  return store.history.filter(h => fuzzyMatch(h.query, searchInput.value)).slice(0, 5)
})

// 是否无任何匹配
const hasResults = computed(() =>
  matchedItems.value.length > 0 ||
  matchedClients.value.length > 0 ||
  matchedHistory.value.length > 0
)

// 各组索引计算
function getGlobalIndex(group, idx) {
  const offsets = {
    favorite: 0,
    history: store.favorites.length,
    item: store.favorites.length + Math.min(store.history.length, 5),
    client: store.favorites.length + Math.min(store.history.length, 5) + matchedItems.value.length,
    matchedHistory: store.favorites.length + Math.min(store.history.length, 5) + matchedItems.value.length + matchedClients.value.length,
  }
  return offsets[group] + idx
}

// 计算总 result 数量
const totalResults = computed(() => {
  if (!searchInput.value) {
    return store.favorites.length + Math.min(store.history.length, 5)
  }
  return matchedItems.value.length + matchedClients.value.length + matchedHistory.value.length
})

function onInput() {
  activeIndex.value = searchInput.value ? 0 : -1
}

function onKeydown(e) {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (activeIndex.value < totalResults.value - 1) activeIndex.value++
    else activeIndex.value = 0
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (activeIndex.value > 0) activeIndex.value--
    else activeIndex.value = totalResults.value - 1
  } else if (e.key === 'Enter') {
    e.preventDefault()
    confirmSelection()
  }
}

function confirmSelection() {
  const idx = activeIndex.value
  if (idx < 0) return

  if (!searchInput.value) {
    // 从收藏 / 历史中选择
    if (idx < store.favorites.length) {
      handleSelectFavorite(store.favorites[idx], idx)
    } else {
      const histIdx = idx - store.favorites.length
      const h = store.history[histIdx]
      if (h) handleSelectHistory(h, histIdx)
    }
    return
  }

  const favCount = store.favorites.length
  const histPreviewCount = Math.min(store.history.length, 5)
  const itemCount = matchedItems.value.length
  const clientCount = matchedClients.value.length

  const totalOffset = favCount + histPreviewCount

  if (idx < totalOffset) {
    // 收藏 / 历史
    if (idx < favCount) handleSelectFavorite(store.favorites[idx], idx)
    else handleSelectHistory(store.history[idx - favCount], idx - favCount)
  } else if (idx < totalOffset + itemCount) {
    handleSelectItem(matchedItems.value[idx - totalOffset], idx - totalOffset)
  } else if (idx < totalOffset + itemCount + clientCount) {
    handleSelectClient(matchedClients.value[idx - totalOffset - itemCount], idx - totalOffset - itemCount)
  } else if (idx < totalOffset + itemCount + clientCount + matchedHistory.value.length) {
    handleSelectMatchedHistory(matchedHistory.value[idx - totalOffset - itemCount - clientCount])
  }
}

function handleSelectFavorite(fav) {
  closeDialog()
  // 跳转到日志检索并执行查询
  router.push({ path: '/logs', query: { key: fav.query, item: fav.params?.item || '' } })
}

function handleSelectHistory(h) {
  closeDialog()
  router.push({ path: '/logs', query: { key: h.query, item: h.params?.item || '' } })
}

function handleSelectMatchedHistory(h) {
  closeDialog()
  router.push({ path: '/logs', query: { key: h.query, item: h.params?.item || '' } })
}

function handleSelectItem(item) {
  closeDialog()
  router.push({ path: '/items', query: { id: String(item.id) } })
}

function handleSelectClient(client) {
  closeDialog()
  router.push({ path: '/clients', query: { id: String(client.id) } })
}

function closeDialog() {
  dialogVisible.value = false
  searchInput.value = ''
  activeIndex.value = -1
}

async function onOpened() {
  store.load()
  await nextTick()
  inputRef.value?.focus()
  // 预加载日志项和客户端列表
  try {
    const [itemsRes, clientsRes] = await Promise.all([queryAllItems(), queryAllClients()])
    items.value = Array.isArray(itemsRes.data) ? itemsRes.data : itemsRes.data?.data || []
    clients.value = Array.isArray(clientsRes.data) ? clientsRes.data : clientsRes.data?.data || []
  } catch {
    /* 网络异常时忽略，搜索功能降级为仅搜索历史 */
  }
}

function onClosed() {
  searchInput.value = ''
  activeIndex.value = -1
}

function open() {
  dialogVisible.value = true
}

defineExpose({ open })
</script>

<style scoped>
.global-search :deep(.el-dialog__body) {
  padding: 16px 20px;
}
.global-search :deep(.el-dialog__header) {
  padding: 0;
}
.result-item {
  transition: background-color 0.15s ease;
}
.result-item:hover {
  background-color: var(--bg-hover, #f3f4f6);
}
kbd {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
}
</style>

<style>
.global-search-dialog {
  border-radius: 12px !important;
}
.global-search-dialog .el-dialog__headerbtn {
  font-size: 18px;
}
</style>
