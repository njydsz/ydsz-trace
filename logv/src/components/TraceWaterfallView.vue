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

      <!-- 瀑布图 (SVG) -->
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
                :class="getSpanClass(span)"
                :style="getSpanStyle(span)"
                :title="`[${span.level}] ${span.timestamp}\n${span.content}`"
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
      <el-table :data="spansWithIndex" stripe size="small" max-height="400">
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
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { traceWaterfall } from '@/api/logs'

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

function getSpanClass(span) {
  if (span.isError) return 'bg-red-500 ring-2 ring-red-300 ring-offset-1'
  switch (span.level) {
    case 'WARN': return 'bg-yellow-500'
    case 'INFO': return 'bg-blue-500'
    case 'DEBUG': return 'bg-gray-400'
    default: return 'bg-indigo-400'
  }
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
    if (data.code && data.code !== '200') {
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
</style>
