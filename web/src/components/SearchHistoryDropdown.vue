<template>
  <div
    v-if="visible"
    class="absolute left-0 top-full mt-1 w-full bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-xl z-50 max-h-80 overflow-y-auto"
  >
    <!-- 收藏列表 -->
    <div v-if="store.favorites.length > 0" class="border-b border-gray-100 dark:border-gray-700 p-2">
      <div class="text-xs font-semibold text-gray-400 px-2 mb-1">收藏</div>
      <div
        v-for="(fav, idx) in store.favorites"
        :key="'fav-' + idx"
        class="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer group"
        @click="handleSelect(fav)"
      >
        <el-icon class="text-yellow-500 flex-shrink-0" @click.stop="handleUnfavorite(fav)"><Star /></el-icon>
        <span class="flex-1 truncate text-sm text-gray-700 dark:text-gray-200">{{ fav.name || fav.query }}</span>
        <span class="text-xs text-gray-400 flex-shrink-0">
          {{ fav.params?.itemName || fav.params?.item || '' }}
        </span>
      </div>
    </div>

    <!-- 历史列表 -->
    <div class="p-2">
      <div class="flex items-center justify-between px-2 mb-1">
        <span class="text-xs font-semibold text-gray-400">搜索历史</span>
        <button
          v-if="store.history.length > 0"
          class="text-xs text-gray-400 hover:text-red-500"
          @click="handleClear"
        >
          清空
        </button>
      </div>
      <div v-if="store.history.length === 0" class="text-center text-sm text-gray-400 py-4">
        暂无搜索历史
      </div>
      <div
        v-for="entry in store.history"
        :key="entry.query + entry.timestamp"
        class="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer group"
        @click="handleSelect(entry)"
      >
        <el-icon class="text-gray-400 flex-shrink-0"><Clock /></el-icon>
        <span class="flex-1 truncate text-sm text-gray-700 dark:text-gray-200">
          {{ entry.query }}
          <span v-if="entry.params?.itemName" class="text-xs text-gray-400 ml-1">{{ entry.params.itemName }}</span>
        </span>
        <span v-if="entry.hitCount" class="text-xs text-gray-400 flex-shrink-0">{{ entry.hitCount }} 条</span>
        <el-icon
          class="text-gray-300 dark:text-gray-500 hover:text-yellow-500 opacity-0 group-hover:opacity-100 flex-shrink-0"
          @click.stop="handleFavorite(entry)"
        ><Star /></el-icon>
        <el-icon
          class="text-gray-300 dark:text-gray-500 hover:text-red-500 opacity-0 group-hover:opacity-100 flex-shrink-0"
          @click.stop="handleRemove(entry)"
        ><Close /></el-icon>
      </div>
    </div>
  </div>
</template>

<script setup>
/**
 * SearchHistoryDropdown — 搜索历史与收藏下拉面板
 * 在搜索框聚焦时显示，每条记录可点击执行查询，
 * 收藏条目带星标，历史条目支持删除与收藏。
 */
import { ref } from 'vue'
import { Clock, Star, Close } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useSearchHistory } from '@/store/searchHistory'

const emit = defineEmits(['select', 'close'])
const store = useSearchHistory()
const visible = ref(false)

function show() {
  store.load()
  visible.value = true
}

function hide() {
  visible.value = false
}

function handleSelect(entry) {
  emit('select', { query: entry.query, params: entry.params || {} })
  hide()
}

function handleFavorite(entry) {
  const added = store.toggleFavorite(entry)
  ElMessage.success(added ? '已收藏' : '取消收藏')
}

function handleUnfavorite(entry) {
  store.toggleFavorite(entry)
  ElMessage.success('取消收藏')
}

function handleRemove(entry) {
  store.remove(entry.query, entry.timestamp)
}

function handleClear() {
  store.clearHistory()
  ElMessage.success('已清空搜索历史')
}

defineExpose({ show, hide })
</script>

<style scoped>
.z-50 {
  z-index: 50;
}
</style>
