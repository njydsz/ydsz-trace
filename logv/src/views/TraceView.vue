<template>
  <div>
    <!-- 统一页面头部 -->
    <div class="page-header mb-5">
      <h2 class="page-header__title" data-testid="trace-title">
        <span class="page-header__icon" style="background: linear-gradient(135deg, #f59e0b, #d97706); box-shadow: 0 2px 8px rgba(245, 158, 11, 0.2);">
          <svg :width="18" :height="18" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M4 12v8a2 2 0 002 2h12a2 2 0 002-2v-8M16 6l-4-4-4 4M12 2v13"/>
          </svg>
        </span>
        日志链路追踪
      </h2>
      <p class="page-header__subtitle" data-testid="trace-subtitle">根据 TraceId 跨节点聚合调用链日志，支持分组视图与瀑布视图</p>
    </div>

    <!-- 搜索卡片 -->
    <div class="rounded-xl bg-white shadow-card border border-gray-200/60 p-5">
      <!-- 表单网格 -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-x-5 gap-y-4">
        <!-- Trace ID -->
        <div class="md:col-span-2">
          <label class="block text-xs font-semibold text-gray-500 mb-1.5">Trace ID</label>
          <input
            v-model="form.traceId"
            type="text"
            placeholder="输入要追踪的 traceId"
            class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg bg-white placeholder:text-gray-400 focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
            data-testid="trace-id-input"
          />
        </div>
        <!-- 日期 -->
        <div>
          <label class="block text-xs font-semibold text-gray-500 mb-1.5">日期</label>
          <input
            v-model="form.date"
            type="date"
            class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
            @change="form.date = $event.target.value.replace(/-/g, '')"
          />
        </div>
        <!-- 客户端 -->
        <div>
          <label class="block text-xs font-semibold text-gray-500 mb-1.5">客户端</label>
          <select
            v-model="form.client"
            class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
            @change="onClientChange"
          >
            <option :value="0">全部客户端（并发检索）</option>
            <option v-for="c in clients" :key="c.id" :value="c.id">{{ c.ip }}:{{ c.port }}</option>
          </select>
        </div>
        <!-- 日志项 -->
        <div>
          <label class="block text-xs font-semibold text-gray-500 mb-1.5">日志项</label>
          <select
            v-model="form.item"
            class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
            :disabled="itemsLoading"
          >
            <option :value="null" disabled>{{ itemsLoading ? '加载中...' : '选择日志项' }}</option>
            <option v-for="it in items" :key="it.id" :value="it.id">{{ it.itemName }}</option>
          </select>
        </div>
      </div>

      <!-- 正则匹配 -->
      <div class="flex items-center gap-3 py-3 mt-2">
        <label class="flex items-center gap-1.5 cursor-pointer">
          <input type="checkbox" v-model="form.regex" class="rounded border-gray-300 text-brand-600 focus:ring-brand-500" />
          <span class="text-sm text-gray-700">正则匹配<span class="text-gray-400 ml-1">把 traceId 作为正则表达式</span></span>
        </label>
      </div>

      <!-- 操作按钮 -->
      <div class="flex items-center gap-3">
        <button
          class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg bg-brand-600 text-white text-sm font-medium hover:bg-brand-700 active:bg-brand-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors shadow-sm"
          :disabled="loading"
          data-testid="trace-submit"
          @click="onTrace"
        >
          <svg v-if="!loading" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 19l-7-7 7-7M15 5l7 7-7 7"/>
          </svg>
          <svg v-else class="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
          </svg>
          追踪调用链
        </button>
        <button
          class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg bg-gray-100 text-gray-700 text-sm font-medium hover:bg-gray-200 transition-colors"
          @click="resetForm"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 102.13-9.36L1 10"/>
          </svg>
          清空
        </button>
      </div>
    </div>

      <!-- 结果卡片 -->
    <div v-if="result" class="rounded-xl bg-white shadow-card border border-gray-200/60 p-5 mt-6" data-testid="trace-result">
      <!-- 自定义 Tab -->
      <div class="border-b border-gray-200 mb-4">
        <nav class="flex gap-6 -mb-px">
          <button
            :class="[
              'pb-3 text-sm font-medium border-b-2 transition-colors',
              resultViewType === 'node'
                ? 'border-brand-600 text-brand-600'
                : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
            ]"
            data-testid="trace-view-node"
            @click="resultViewType = 'node'"
          >
            节点分组
          </button>
          <button
            :class="[
              'pb-3 text-sm font-medium border-b-2 transition-colors',
              resultViewType === 'waterfall'
                ? 'border-brand-600 text-brand-600'
                : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
            ]"
            data-testid="trace-view-waterfall"
            @click="resultViewType = 'waterfall'"
          >
            瀑布视图
          </button>
        </nav>
      </div>

      <!-- 节点分组视图 -->
      <div v-if="resultViewType === 'node'">
        <div class="text-sm text-gray-600 mb-4">
          TraceId
          <code class="bg-gray-100 px-1.5 py-0.5 rounded mx-1">{{ result.traceId }}</code>
          共 {{ result.Total }} 行命中 / 涉及 {{ result.Nodes }} 个节点 / 展示 {{ result.Results.length }} 个节点
        </div>

        <!-- 空状态 -->
        <div v-if="result.Results.length === 0" class="text-center py-12">
          <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9.75 9.75l4.5 4.5m0-4.5l-4.5 4.5M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
          </svg>
          <p class="mt-2 text-sm text-gray-500">未找到匹配 traceId 的日志行</p>
        </div>

        <!-- 分组展示 -->
        <div v-for="group in result.Results" :key="group.Node" class="mb-6">
          <div class="flex items-center gap-2 mb-2">
            <span class="inline-flex items-center rounded-full bg-brand-100 text-brand-700 px-2.5 py-0.5 text-xs font-semibold">
              {{ group.Node }}
            </span>
            <span class="text-sm text-gray-500">{{ group.Count }} 行</span>
          </div>
          <div class="bg-gray-900 rounded-lg p-3 max-h-[400px] overflow-y-auto font-mono text-xs">
            <div
              v-for="(line, idx) in group.Lines"
              :key="idx"
              class="log-line"
              :class="getLineClass(line)"
            >
              <span class="text-gray-500 mr-2 select-none">{{ idx + 1 }}</span>
              <span>{{ line }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 瀑布视图 -->
      <div v-if="resultViewType === 'waterfall'">
        <TraceWaterfallView
          :trace-id="form.traceId.trim()"
          :date="form.date"
          :item="form.item"
          :client="form.client"
          :max-per-node="5000"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { queryLogClients, queryItemsByClient, traceLogs } from '@/api/logs'
import TraceWaterfallView from '@/components/TraceWaterfallView.vue'

const route = useRoute()

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const loading = ref(false)
const result = ref(null)
const resultViewType = ref('node')

const form = reactive({
  traceId: '',
  date: '',
  client: 0,
  item: null,
  regex: false,
})

onMounted(() => {
  loadClients()
  applyRouteQuery()
})

function applyRouteQuery() {
  const q = route.query
  if (!q) return
  if (q.traceId) form.traceId = String(q.traceId)
  if (q.date) form.date = String(q.date)
  if (q.client) form.client = Number(q.client)
  if (q.item) form.item = Number(q.item)
  if (q.regex === '1' || q.regex === 'true') form.regex = true
  // 如果 URL query 已包含所有必要参数，自动发起追踪
  if (form.traceId && form.date && form.item) {
    onTrace()
  }
}

async function loadClients() {
  try {
    const { data } = await queryLogClients()
    clients.value = data.data || []
  } catch {
    /* ignore */
  }
}

async function onClientChange() {
  form.item = null
  itemsLoading.value = true
  try {
    if (form.client && form.client !== 0) {
      const { data } = await queryItemsByClient(form.client)
      items.value = data.data || []
    } else {
      items.value = []
    }
  } catch {
    items.value = []
  } finally {
    itemsLoading.value = false
  }
}

function resetForm() {
  form.traceId = ''
  form.date = ''
  form.client = 0
  form.item = null
  form.regex = false
  result.value = null
}

async function onTrace() {
  if (!form.traceId.trim()) {
    ElMessage.warning('请输入 traceId')
    return
  }
  if (!form.date) {
    ElMessage.warning('请选择日期')
    return
  }
  if (!form.item) {
    ElMessage.warning('请选择日志项')
    return
  }
  loading.value = true
  result.value = null
  try {
    const { data } = await traceLogs({
      traceId: form.traceId.trim(),
      date: form.date,
      client: form.client || 0,
      item: form.item,
      regex: form.regex,
    })
    if (data.code && data.code !== 0) {
      ElMessage.error(data.msg || '追踪失败')
      return
    }
    result.value = data.data || { traceId: form.traceId, Nodes: 0, Total: 0, Results: [] }
    if (result.value.Total === 0) {
      ElMessage.info('未找到匹配该 traceId 的日志行')
    } else {
      ElMessage.success(`追踪完成，共 ${result.value.Total} 行`)
    }
  } catch {
    ElMessage.error('追踪请求失败，请确认服务端已启动')
  } finally {
    loading.value = false
  }
}

function getLineClass(line) {
  if (!line) return ''
  const upper = line.toUpperCase()
  if (upper.includes('ERROR') || upper.includes('FATAL')) return 'text-red-400'
  if (upper.includes('WARN')) return 'text-yellow-400'
  if (upper.includes('DEBUG')) return 'text-gray-400'
  if (upper.includes('INFO')) return 'text-green-400'
  return 'text-gray-200'
}
</script>

<style scoped>
.log-line {
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.5;
}
</style>
