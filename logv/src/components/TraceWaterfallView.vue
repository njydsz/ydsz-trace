<template>
  <div class="trace-waterfall">
    <!-- 加载中 -->
    <div v-if="loading" class="flex justify-center py-8">
      <el-icon class="animate-spin text-2xl text-primary-500"><Loading /></el-icon>
      <span class="ml-2 text-gray-500">正在检索瀑布数据...</span>
    </div>

    <!-- 错误提示 -->
    <el-alert
      v-if="errorMsg"
      type="error"
      :closable="false"
      class="mb-4"
    >
      <template #title>
        {{ errorMsg }}
      </template>
    </el-alert>

    <!-- 空状态 -->
    <el-empty
      v-if="!loading && !errorMsg && spans.length === 0 && hasSearched"
      description="未找到匹配 traceId 的日志行"
    />

    <!-- 瀑布视图内容 -->
    <div v-if="!loading && spans.length > 0">
      <!-- 概要信息 -->
      <div class="flex flex-wrap gap-4 mb-4 text-sm text-gray-600">
        <el-tag size="small">格式: {{ waterfallData.traceFormat || 'unknown' }}</el-tag>
        <el-tag size="small" type="primary">节点: {{ waterfallData.totalNodes || 0 }}</el-tag>
        <el-tag size="small" type="info">Spans: {{ spans.length }}</el-tag>
        <el-tag v-if="waterfallData.errorCount > 0" size="small" type="danger">
          错误: {{ waterfallData.errorCount }}
        </el-tag>
      </div>

      <!-- 首个错误提示 -->
      <el-alert
        v-if="waterfallData.firstError"
        type="error"
        :closable="false"
        class="mb-4"
      >
        <template #title>
          首个错误: [{{ waterfallData.firstError.level }}] {{ waterfallData.firstError.node }}
        </template>
        <div class="text-xs text-gray-600 font-mono mt-1 break-all">
          {{ waterfallData.firstError.content }}
        </div>
      </el-alert>

      <!-- 概要统计卡片 -->
      <div v-if="waterfallData.durationMs > 0 || waterfallData.longestGapMs > 0" class="flex flex-wrap gap-3 mb-4">
        <div v-if="waterfallData.durationMs > 0" class="stat-card">
          <span class="text-xs text-muted">总耗时</span>
          <span class="text-sm font-semibold text-primary">{{ formatDuration(waterfallData.durationMs) }}</span>
        </div>
        <div v-if="waterfallData.longestGapMs > 0" class="stat-card border-orange-200 bg-orange-50">
          <span class="text-xs text-orange-600">最长 span</span>
          <span class="text-sm font-semibold text-orange-600">{{ formatDuration(waterfallData.longestGapMs) }}</span>
        </div>
      </div>

      <!-- 瀑布图 -->
      <div class="waterfall-chart mb-4 overflow-x-auto">
        <div class="min-w-[800px]">
          <!-- 时间轴标签 -->
          <div class="flex items-center h-6 relative border-b border-gray-200 ml-[140px]">
            <span v-for="tick in timeTicks" :key="tick.label"
                  class="absolute text-xs text-gray-400 -translate-x-1/2"
                  :style="{ left: tick.position + '%' }">
              {{ tick.label }}
            </span>
          </div>

          <!-- 节点行 -->
          <div v-for="(nodeRow, rowIdx) in nodeRows" :key="nodeRow.nodeIdx"
               class="flex items-start border-b border-gray-100 hover:bg-gray-50">
            <!-- 节点名单元格 -->
            <div class="w-[140px] flex-shrink-0 px-2 py-2 text-xs text-gray-600 font-mono border-r border-gray-200">
              <div class="truncate" :title="nodes[rowIdx]">{{ nodes[rowIdx] }}</div>
            </div>

            <!-- 时间线区域 -->
            <div class="flex-1 relative h-[36px]">
              <div
                v-for="span in nodeRow.spans"
                :key="span._key"
                class="absolute h-[20px] top-[8px] rounded cursor-pointer transition-all duration-200 hover:scale-y-125"
                :class="getSpanClass(span, span._key)"
                :style="getSpanStyle(span)"
                :title="`[${span.level}] ${span.timestamp}\n${span.content}`"
                @click="handleSpanClick(span)"
              >
                <span v-if="getSpanWidth(span) > 40" class="px-1 text-white text-[10px] truncate block leading-[20px]">
                  {{ span.level }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Span 列表 -->
      <el-table :data="spansWithIndex" stripe size="small" max-height="400" :row-class-name="getRowClassName">
        <el-table-column type="index" label="#" width="55">
          <template #default="{ $index }">
            <span :class="{ 'animate-pulse font-bold text-red-500': spansWithIndex[$index].isFirstError }">
              {{ $index + 1 }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="timestamp" label="时间戳" width="200" show-overflow-tooltip />
        <el-table-column prop="level" label="级别" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="getLevelTagType(row.level)">{{ row.level || '-' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="node" label="节点" width="160" show-overflow-tooltip />
        <el-table-column prop="service" label="服务" width="120" show-overflow-tooltip />
        <el-table-column prop="thread" label="线程" width="120" show-overflow-tooltip />
        <el-table-column prop="content" label="摘要" min-width="300" show-overflow-tooltip />
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button link size="small" type="primary" @click="handleSpanClick(row)">上下文</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 日志上下文对话框 -->
    <el-dialog v-model="contextDialogVisible" :title="`日志上下文 — ${contextSpan?.node || ''}`" width="70%">
      <div v-if="contextLoading" class="flex justify-center py-4">
        <el-icon class="animate-spin text-2xl text-primary-500"><Loading /></el-icon>
        <span class="ml-2 text-gray-500">加载中...</span>
      </div>
      <div v-else-if="contextLines.length > 0" class="bg-gray-900 rounded p-3 max-h-[400px] overflow-y-auto font-mono text-xs">
        <div
          v-for="(line, idx) in contextLines"
          :key="idx"
          class="log-line"
          :class="getContextLineClass(line, idx)"
        >
          <span class="text-gray-500 mr-2 select-none">{{ idx + 1 }}</span>
          <span>{{ line }}</span>
        </div>
      </div>
      <el-empty v-else description="未找到上下文日志，可能是本地未缓存该节点的日志文件" />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { traceWaterfall, logContext } from '@/api/logs'

const props = defineProps({
  traceId: { type: String, required: true },
  date: { type: String, required: true },
  item: { type: [Number, String], required: true },
  client: { type: [Number, String], default: 0 },
  maxPerNode: { type: Number, default: 5000 },
})

const loading = ref(false)
const errorMsg = ref('')
const waterfallData = ref({})
const spans = ref([])
const hasSearched = ref(false)
const contextDialogVisible = ref(false)
const contextLines = ref([])
const contextLoading = ref(false)
const contextSpan = ref(null)

// 计算节点列表（去重）
const nodes = computed(() => {
  const set = new Set()
  for (const s of spans.value) {
    set.add(s.node)
  }
  return Array.from(set)
})

// 按节点分组 spans
const nodeRows = computed(() => {
  const nodeIdxMap = {}
  nodes.value.forEach((n, i) => { nodeIdxMap[n] = i })
  const rows = []
  for (const s of spans.value) {
    const nodeIdx = nodeIdxMap[s.node]
    if (!rows.find(r => r.nodeIdx === nodeIdx)) {
      rows.push({ nodeIdx, spans: [] })
    }
    const row = rows.find(r => r.nodeIdx === nodeIdx)
    row.spans.push(s)
  }
  return rows
})

// 时间范围
const timeRange = computed(() => {
  if (spans.value.length === 0) return { min: 0, max: 0 }
  let min = Infinity
  let max = -Infinity
  for (const s of spans.value) {
    const ts = new Date(s.timestamp).getTime()
    if (!isNaN(ts)) {
      if (ts < min) min = ts
      if (ts > max) max = ts
    }
  }
  if (min === Infinity) return { min: 0, max: 0 }
  return { min, max }
})

// 时间刻度
const timeTicks = computed(() => {
  const { min, max } = timeRange.value
  if (min === 0 && max === 0) return []
  if (min === max) return [{ label: new Date(min).toLocaleTimeString(), position: 50 }]
  const count = 5
  const ticks = []
  for (let i = 0; i < count; i++) {
    const ts = min + (max - min) * i / (count - 1)
    const position = (i / (count - 1)) * 100
    const d = new Date(ts)
    const label = `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}:${d.getSeconds().toString().padStart(2, '0')}`
    ticks.push({ label, position })
  }
  return ticks
})

// spans with index and isFirstError
const spansWithIndex = computed(() => {
  const firstErrId = waterfallData.value?.firstError?.timestamp + waterfallData.value?.firstError?.node
  return spans.value.map((s, i) => ({
    ...s,
    _key: i,
    isFirstError: (s.timestamp + s.node) === firstErrId && s.isError,
  }))
})

function getSpanClass(span, spanIdx) {
  const classes = []
  // 最长耗时 span：用橙色描边高亮
  if (waterfallData.value.longestSpanIdx === spanIdx) {
    classes.push('longest-span')
  }
  if (span.isError) {
    classes.push('bg-red-500 ring-2 ring-red-300 ring-offset-1')
  } else {
    switch (span.level) {
      case 'WARN': classes.push('bg-yellow-500'); break
      case 'INFO': classes.push('bg-blue-500'); break
      case 'DEBUG': classes.push('bg-gray-400'); break
      default: classes.push('bg-indigo-400')
    }
  }
  return classes.join(' ')
}

function getSpanStyle(span) {
  const { min, max } = timeRange.value
  const ts = new Date(span.timestamp).getTime()
  if (isNaN(ts) || min === max) {
    return { left: '0%', width: '100%' }
  }
  const pct = ((ts - min) / (max - min)) * 100
  return {
    left: Math.max(0, Math.min(95, pct)) + '%',
    width: '6px',
  }
}

function getSpanWidth(span) {
  // 用于判断 span 内部是否展示文字
  const { min, max } = timeRange.value
  const ts = new Date(span.timestamp).getTime()
  if (isNaN(ts) || min === max) return 100
  return 60 // 固定最小宽度以便展示
}

function getLevelTagType(level) {
  switch (level) {
    case 'ERROR':
    case 'FATAL':
      return 'danger'
    case 'WARN':
      return 'warning'
    case 'INFO':
      return 'success'
    case 'DEBUG':
      return 'info'
    default:
      return ''
  }
}

// formatDuration 将毫秒格式化为可读字符串。
function formatDuration(ms) {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(2)}s`
  const min = Math.floor(ms / 60000)
  const sec = ((ms % 60000) / 1000).toFixed(1)
  return `${min}m${sec}s`
}

// getRowClassName 为最长耗时 span 所在行添加高亮类。
function getRowClassName({ row }) {
  if (waterfallData.value.longestSpanIdx === row._key) return 'longest-row'
  return ''
}

// getContextLineClass 为上下文日志行着色。
function getContextLineClass(line, idx) {
  const total = contextLines.value.length
  const refIdx = Math.floor(total / 2)
  if (idx === refIdx) return 'bg-yellow-900/30 text-yellow-200'
  const upper = line.toUpperCase()
  if (upper.includes('ERROR') || upper.includes('FATAL')) return 'text-red-400'
  if (upper.includes('WARN')) return 'text-yellow-400'
  return 'text-gray-300'
}

// handleSpanClick 点击 span 后调用日志上下文接口，弹出对话框展示前后日志。
async function handleSpanClick(span) {
  contextSpan.value = span
  contextLines.value = []
  contextLoading.value = true
  contextDialogVisible.value = true
  try {
    const { data: resp } = await logContext(Number(props.item), {
      clientId: Number(props.client || 0),
      refTime: span.timestamp,
      date: props.date,
      before: 10,
      after: 10,
    })
    if (resp.code === 0 && resp.data?.list) {
      contextLines.value = resp.data.list
    }
  } catch {
    contextLines.value = []
  } finally {
    contextLoading.value = false
  }
}

async function fetchWaterfall() {
  if (!props.traceId || !props.date || !props.item) return
  loading.value = true
  errorMsg.value = ''
  spans.value = []
  waterfallData.value = {}
  try {
    const { data } = await traceWaterfall({
      traceId: props.traceId.trim(),
      date: props.date,
      item: Number(props.item),
      client: Number(props.client || 0),
      maxPerNode: props.maxPerNode,
    })
    if (data.code && data.code !== 0) {
      errorMsg.value = data.msg || '查询失败'
      return
    }
    const resp = data.data || {}
    waterfallData.value = resp
    spans.value = resp.spans || []
    hasSearched.value = true
  } catch {
    errorMsg.value = '检索失败，请确认服务端已启动'
  } finally {
    loading.value = false
  }
}

watch(() => [props.traceId, props.date, props.item], () => {
  if (props.traceId && props.date && props.item) {
    fetchWaterfall()
  }
}, { immediate: false })

onMounted(() => {
  if (props.traceId && props.date && props.item) {
    fetchWaterfall()
  }
})
</script>

<style scoped>
.trace-waterfall {
  min-height: 200px;
}

.waterfall-chart {
  position: relative;
}

/* 最长耗时 span：橙色描边高亮 */
.longest-span {
  outline: 2px solid #f97316;
  outline-offset: 1px;
  z-index: 5;
}

/* 概要统计卡片 */
.stat-card {
  display: flex;
  flex-direction: column;
  padding: 8px 14px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  background: #f9fafb;
}

/* 表格最长行高亮 :deep 穿透 scoped */
:deep(.longest-row) {
  --el-table-tr-bg-color: #fff7ed;
}
:deep(.longest-row td) {
  background-color: #fff7ed !important;
}
</style>
