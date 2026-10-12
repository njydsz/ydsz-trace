<template>
  <div class="log-histogram">
    <div class="flex items-center justify-between mb-2">
      <span class="text-sm text-gray-600">时间分布直方图（共 {{ totalCount }} 条）</span>
      <div class="flex items-center gap-3">
        <span
          v-if="brushRange"
          class="text-xs text-blue-600"
        >
          已选择: {{ brushRange.start }} ~ {{ brushRange.end }}
        </span>
        <el-button
          v-if="brushRange"
          size="small"
          type="primary"
          @click="handleConfirmBrush"
        >
          应用时间过滤
        </el-button>
        <el-button
          v-if="brushRange"
          size="small"
          @click="handleClearBrush"
        >
          清除选择
        </el-button>
      </div>
    </div>

    <!-- SVG 直方图 -->
    <svg
      ref="svgRef"
      :width="width"
      :height="height"
      class="select-none"
      @mousedown="handleMouseDown"
      @mousemove="handleMouseMove"
      @mouseup="handleMouseUp"
      @mouseleave="handleMouseUp"
    >
      <!-- Y 轴网格线 -->
      <line
        v-for="grid in yGrids"
        :key="grid.key"
        :x1="padding.left"
        :y1="grid.y"
        :x2="width - padding.right"
        :y2="grid.y"
        stroke="#ebeef5"
        stroke-dasharray="3,3"
      />
      <!-- Y 轴刻度 -->
      <text
        v-for="tick in yTicks"
        :key="tick.key"
        :x="padding.left - 5"
        :y="tick.y + 4"
        text-anchor="end"
        font-size="10"
        fill="#909399"
      >
        {{ tick.label }}
      </text>

      <!-- 堆叠柱 -->
      <g v-for="(bar, idx) in bars" :key="idx">
        <rect
          v-for="seg in bar.segments"
          :key="seg.level"
          :x="bar.x"
          :y="seg.y"
          :width="barWidth"
          :height="seg.height"
          :fill="seg.color"
          :opacity="0.85"
          stroke="#fff"
          stroke-width="0.5"
        >
          <title>{{ seg.level }}: {{ seg.count }} 条 ({{ bar.timeLabel }})</title>
        </rect>
      </g>

      <!-- X 轴刻度 -->
      <text
        v-for="(tick, idx) in xTicks"
        :key="'xt' + idx"
        :x="padding.left + tick.pos * barAreaWidth"
        :y="height - padding.bottom + 16"
        text-anchor="middle"
        font-size="10"
        fill="#909399"
      >
        {{ tick.label }}
      </text>

      <!-- 刷选遮罩 -->
      <!-- 左侧遮罩 -->
      <rect
        v-if="brushRange && isBrushing"
        :x="padding.left"
        :y="padding.top"
        :width="(brushStartX - padding.left)"
        :height="barAreaHeight"
        fill="rgba(64,158,255,0.08)"
      />
      <!-- 右侧遮罩 -->
      <rect
        v-if="brushRange && isBrushing"
        :x="brushEndX"
        :y="padding.top"
        :x2="width - padding.right"
        :width="width - padding.right - brushEndX"
        :height="barAreaHeight"
        fill="rgba(64,158,255,0.08)"
      />
      <!-- 刷选区间 -->
      <rect
        v-if="brushRange && isBrushing"
        :x="Math.min(brushStartX, brushEndX)"
        :y="padding.top"
        :width="Math.abs(brushEndX - brushStartX)"
        :height="barAreaHeight"
        fill="rgba(64,158,255,0.2)"
        stroke="#409EFF"
        stroke-width="1"
      />
      <!-- 刷选边界线 -->
      <line
        v-if="brushRange && isBrushing"
        :x1="brushStartX"
        :y1="padding.top"
        :x2="brushStartX"
        :y2="height - padding.bottom"
        stroke="#409EFF"
        stroke-width="1.5"
        stroke-dasharray="4,2"
      />
      <line
        v-if="brushRange && isBrushing"
        :x1="brushEndX"
        :y1="padding.top"
        :x2="brushEndX"
        :y2="height - padding.bottom"
        stroke="#409EFF"
        stroke-width="1.5"
        stroke-dasharray="4,2"
      />
    </svg>

    <!-- 图例 -->
    <div class="flex gap-4 mt-2 flex-wrap">
      <span
        v-for="level in levelColors"
        :key="level.name"
        class="flex items-center gap-1 text-xs text-gray-600"
      >
        <span :style="{ width: '12px', height: '12px', background: level.color, display: 'inline-block', borderRadius: '2px' }" />
        {{ level.name }} ({{ levelCounts[level.name] || 0 }})
      </span>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'

const props = defineProps({
  rows: {
    type: Array,
    required: true,
  },
})

const emit = defineEmits(['update'])

const width = 720
const height = 180
const padding = { top: 16, right: 16, bottom: 28, left: 48 }

const barAreaWidth = computed(() => width - padding.left - padding.right)
const barAreaHeight = computed(() => height - padding.top - padding.bottom)

// 级别颜色映射
const levelColors = [
  { name: 'DEBUG', color: '#909399' },
  { name: 'INFO', color: '#409EFF' },
  { name: 'WARN', color: '#E6A23C' },
  { name: 'ERROR', color: '#F56C6C' },
  { name: 'FATAL', color: '#FF0000' },
]

const levelMap = levelColors.reduce((acc, item) => {
  acc[item.name] = item.color
  return acc
}, {})

// 解析行数据，提取 timestamp 和 level
function parseRow(row) {
  // 如果行是字符串格式
  if (typeof row === 'string') {
    const tsMatch = row.match(/^\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}/)
    const levelMatch = row.match(/\b(DEBUG|INFO|WARN|ERROR|FATAL)\b/i)
    return {
      timestamp: tsMatch ? tsMatch[0] : null,
      level: levelMatch ? levelMatch[1].toUpperCase() : 'INFO',
      raw: row,
    }
  }
  // 对象格式：优先使用 row.timestamp 和 row.level
  const ts = row.timestamp || row.time || row.ts || ''
  const lvl = row.level || row.logLevel || inferLevelFromContent(row.line || row.content || '')
  // 尝试从 raw line 中提取时间戳
  const tsValue = parseTimestamp(ts, row.line || row.content || '')
  return {
    timestamp: tsValue,
    level: normalizeLevel(lvl),
  }
}

function inferLevelFromContent(content) {
  if (!content || typeof content !== 'string') return 'INFO'
  const upper = content.toUpperCase()
  if (upper.includes('FATAL')) return 'FATAL'
  if (upper.includes('ERROR')) return 'ERROR'
  if (upper.includes('WARN')) return 'WARN'
  if (upper.includes('DEBUG')) return 'DEBUG'
  return 'INFO'
}

function normalizeLevel(level) {
  const l = String(level || '').toUpperCase()
  return levelMap[l] ? l : 'INFO'
}

function parseTimestamp(ts, fallback) {
  if (ts && typeof ts === 'string') {
    // 尝试解析 ISO 格式时间戳
    const d = new Date(ts)
    if (!isNaN(d.getTime())) return d.getTime()
  }
  if (ts && typeof ts === 'number') return ts
  // 尝试从 fallback 中提取时间戳
  if (typeof fallback === 'string') {
    const m = fallback.match(/(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?)/)
    if (m) {
      const d = new Date(m[1])
      if (!isNaN(d.getTime())) return d.getTime()
    }
  }
  return null
}

// 总日志计数
const totalCount = computed(() => props.rows.length)

// 级别计数（用于图例）
const levelCounts = computed(() => {
  const counts = {}
  for (const row of props.rows) {
    const parsed = parseRow(row)
    counts[parsed.level] = (counts[parsed.level] || 0) + 1
  }
  return counts
})

// 计算直方图数据
const histogramData = computed(() => {
  // 解析所有行数据
  const parsed = props.rows.map(parseRow)
  const valid = parsed.filter(p => p.timestamp !== null)
  if (valid.length === 0) return null

  const timestamps = valid.map(p => p.timestamp)
  const minTs = Math.min(...timestamps)
  const maxTs = Math.max(...timestamps)
  if (minTs >= maxTs) return null

  // 计算最优 bin 数量
  const rangeMs = maxTs - minTs
  const dayMs = 86400000
  const hourMs = 3600000
  const minuteMs = 60000
  let binCount
  if (rangeMs > dayMs * 3) {
    binCount = 60
  } else if (rangeMs > dayMs) {
    binCount = 48
  } else if (rangeMs > hourMs * 6) {
    binCount = 30
  } else {
    binCount = Math.min(60, Math.max(30, Math.floor(rangeMs / minuteMs / 2)))
  }

  const binSize = rangeMs / binCount
  // 初始化 bins
  const bins = Array.from({ length: binCount }, (_, i) => ({
    startTime: minTs + i * binSize,
    endTime: minTs + (i + 1) * binSize,
    counts: { DEBUG: 0, INFO: 0, WARN: 0, ERROR: 0, FATAL: 0 },
    total: 0,
  }))

  // 分桶统计
  for (const item of valid) {
    let binIdx = Math.floor((item.timestamp - minTs) / binSize)
    if (binIdx >= binCount) binIdx = binCount - 1
    if (binIdx < 0) binIdx = 0
    bins[binIdx].counts[item.level]++
    bins[binIdx].total++
  }

  return { bins, minTs, maxTs, binSize }
})

// max value for Y axis
const maxY = computed(() => {
  if (!histogramData.value) return 1
  const max = Math.max(...histogramData.value.bins.map(b => b.total), 1)
  return max
})

// Y 轴刻度和网格
const yTicks = computed(() => {
  if (!histogramData.value) return []
  const count = maxY.value
  const step = niceStep(count, 5)
  const result = []
  for (let v = 0; v <= count; v += step) {
    const y = padding.top + barAreaHeight.value - (v / count) * barAreaHeight.value
    result.push({ y, label: String(v) })
  }
  return result
})

const yGrids = computed(() => yTicks.value)

function niceStep(max, targetCount) {
  if (max <= 0) return 1
  const rough = max / targetCount
  const pow = Math.pow(10, Math.floor(Math.log10(rough)))
  const normalized = rough / pow
  let nice
  if (normalized <= 1) nice = 1
  else if (normalized <= 2) nice = 2
  else if (normalized <= 5) nice = 5
  else nice = 10
  return nice * pow
}

// X 轴刻度
const xTicks = computed(() => {
  if (!histogramData.value) return []
  const { minTs, maxTs } = histogramData.value
  const count = 6
  const step = (maxTs - minTs) / count
  const fmt = formatTimeAxis(minTs, maxTs)
  const result = []
  for (let i = 0; i <= count; i++) {
    result.push({
      pos: i / count,
      label: fmt(minTs + i * step),
    })
  }
  return result
})

function formatTimeAxis(minTs, maxTs) {
  const rangeMs = maxTs - minTs
  if (rangeMs > 86400000) {
    return (ts) => {
      const d = new Date(ts)
      return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours()}时`
    }
  }
  return (ts) => {
    const d = new Date(ts)
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
}

function pad(n) { return String(n).padStart(2, '0') }

// 柱状图数据
const bars = computed(() => {
  if (!histogramData.value) return []
  const { bins } = histogramData.value
  const count = maxY.value || 1
  const bw = barAreaWidth.value / bins.length
  const gap = 1

  return bins.map((bin, i) => {
    let currentY = padding.top + barAreaHeight.value
    const segments = []
    for (const levelInfo of levelColors) {
      const lvl = levelInfo.name
      const c = bin.counts[lvl]
      if (c > 0) {
        const h = (c / count) * barAreaHeight.value
        currentY -= h
        segments.push({
          level: lvl,
          color: levelInfo.color,
          count: c,
          y: currentY,
          height: h,
        })
      }
    }
    return {
      x: padding.left + i * bw + gap / 2,
      width: bw - gap,
      segments,
      timeLabel: formatBinTime(bin),
    }
  })
})

function formatBinTime(bin) {
  const start = new Date(bin.startTime)
  const h = pad(start.getHours())
  const m = pad(start.getMinutes())
  return `${h}:${m}`
}

// 刷选逻辑
const svgRef = ref(null)
const brushStartX = ref(0)
const brushEndX = ref(0)
const isBrushing = ref(false)
const brushRange = ref(null)

function getSvgX(e) {
  const svg = svgRef.value
  if (!svg) return 0
  const rect = svg.getBoundingClientRect()
  return ((e.clientX - rect.left) / rect.width) * width
}

function handleMouseDown(e) {
  e.preventDefault()
  const x = getSvgX(e)
  brushStartX.value = x
  brushEndX.value = x
  isBrushing.value = true
  brushRange.value = null
}

function handleMouseMove(e) {
  if (!isBrushing.value) return
  brushEndX.value = getSvgX(e)
}

function handleMouseUp() {
  if (!isBrushing.value) return
  isBrushing.value = false

  const startX = Math.min(brushStartX.value, brushEndX.value)
  const endX = Math.max(brushStartX.value, brushEndX.value)

  // 忽略太小的选区
  if (endX - startX < 4) {
    brushRange.value = null
    return
  }

  if (!histogramData.value) return

  const { minTs, maxTs } = histogramData.value
  const timeRange = maxTs - minTs
  const startRatio = Math.max(0, (startX - padding.left) / barAreaWidth.value)
  const endRatio = Math.min(1, (endX - padding.left) / barAreaWidth.value)

  const startTs = minTs + startRatio * timeRange
  const endTs = minTs + endRatio * timeRange

  const fmt = (ts) => {
    const d = new Date(ts)
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  }

  brushRange.value = {
    start: fmt(startTs),
    end: fmt(endTs),
    startTs,
    endTs,
  }
}

function handleConfirmBrush() {
  if (!brushRange.value) return
  const { startTs, endTs } = brushRange.value
  // 格式化为 HH:mm:ss
  const fmt = (ts) => {
    const d = new Date(ts)
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  }
  const result = {
    startTime: fmt(startTs),
    endTime: fmt(endTs),
  }
  emit('update', result)
}

function handleClearBrush() {
  brushRange.value = null
}

// 监听 rows 变化时清除刷选
watch(() => props.rows, () => {
  brushRange.value = null
})
</script>

<style scoped>
.log-histogram {
  position: relative;
  padding: 12px;
  background: #fafafa;
  border-radius: 6px;
  border: 1px solid #ebeef5;
}
svg {
  display: block;
}
</style>
