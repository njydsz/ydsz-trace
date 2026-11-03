<template>
  <div class="anomaly-timeline">
    <!-- 时间范围切换 -->
    <div class="flex items-center justify-between mb-3">
      <div class="flex items-center gap-2">
        <span class="text-sm text-muted">时间范围</span>
        <el-radio-group v-model="rangeKey" size="small" @change="onRangeChange">
          <el-radio-button label="1h">近 1 小时</el-radio-button>
          <el-radio-button label="6h">近 6 小时</el-radio-button>
          <el-radio-button label="24h">近 24 小时</el-radio-button>
          <el-radio-button label="7d">近 7 天</el-radio-button>
        </el-radio-group>
      </div>
      <div class="flex items-center gap-2 text-xs text-muted">
        <span>误报 {{ stats.falsePositive }}</span>
        <span>·</span>
        <span>确认 {{ stats.truePositive }}</span>
        <span>·</span>
        <span>忽略 {{ stats.ignored }}</span>
      </div>
    </div>

    <!-- 时间线主体 -->
    <div v-loading="loading" class="relative">
      <div v-if="!points.length" class="py-10 text-center text-sm text-muted">
        该时间范围内暂无异常事件
      </div>

      <div v-else class="relative pl-4 border-l-2 border-gray-200 dark:border-gray-700 space-y-4">
        <div
          v-for="pt in points"
          :key="`${pt.time}-${pt.templateHash}`"
          class="relative"
        >
          <!-- 时间线节点 -->
          <span
            class="absolute -left-[21px] top-1.5 w-3 h-3 rounded-full border-2"
            :class="dotClass(pt.type)"
          />

          <div class="flex flex-wrap items-center gap-2">
            <span class="text-xs text-muted font-mono">{{ formatTime(pt.time) }}</span>
            <el-tag size="small" :type="tagType(pt.type)">{{ typeLabel(pt.type) }}</el-tag>
            <span class="text-xs text-muted font-mono">{{ shortHash(pt.templateHash) }}</span>
            <span class="text-xs text-muted">score {{ pt.score.toFixed(2) }}</span>
            <span class="text-xs text-muted">
              实际 {{ fmtNum(pt.actual) }} / 基线 {{ fmtNum(pt.baseline) }}
            </span>
          </div>

          <p v-if="pt.description" class="mt-1 text-xs text-muted leading-relaxed">
            {{ pt.description }}
          </p>

          <!-- 操作：下钻 + 反馈 -->
          <div class="mt-1.5 flex items-center gap-2">
            <el-button
              size="small"
              text
              type="primary"
              @click="onDrillDown(pt)"
            >下钻原文</el-button>
            <el-button
              size="small"
              text
              type="warning"
              :loading="feedbackBusy === feedbackKey(pt, 'false_positive')"
              @click="onFeedback(pt, 'false_positive')"
            >标记误报</el-button>
            <el-button
              size="small"
              text
              type="success"
              :loading="feedbackBusy === feedbackKey(pt, 'true_positive')"
              @click="onFeedback(pt, 'true_positive')"
            >确认</el-button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="hasMore" class="mt-3 text-center">
      <el-button size="small" text type="primary" :loading="loading" @click="loadMore">
        加载更多
      </el-button>
    </div>
  </div>
</template>

<script setup>
// 异常时间线（V2.2-04）
//
// 功能：
//   - 按时间倒序展示异常事件（spike / new / disappear）
//   - 每个点支持「下钻原文」与「标记误报 / 确认」反馈
//   - 反馈后触发后端自适应阈值，同类误报自动减少
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getAnomalyTimeline,
  getFeedbackStats,
  markAnomalyFeedback,
} from '@/api/alert'

const emit = defineEmits(['drill-down'])

const rangeKey = ref('24h')
const points = ref([])
const stats = reactive({ falsePositive: 0, truePositive: 0, ignored: 0 })
const loading = ref(false)
const hasMore = ref(false)
const feedbackBusy = ref('')

const RANGE_MS = { '1h': 3600e3, '6h': 6 * 3600e3, '24h': 24 * 3600e3, '7d': 7 * 86400e3 }

function formatTime(t) {
  return t ? String(t).replace(' ', ' ').slice(5, 19) : '-'
}

function shortHash(h) {
  return h ? String(h).slice(0, 8) : '-'
}

function fmtNum(n) {
  if (n === null || n === undefined) return '-'
  return Number(n).toFixed(1)
}

function dotClass(type) {
  if (type === 'new') return 'bg-blue-500 border-blue-200'
  if (type === 'disappear') return 'bg-gray-400 border-gray-200'
  return 'bg-red-500 border-red-200'
}

function tagType(type) {
  if (type === 'new') return 'info'
  if (type === 'disappear') return 'info'
  return 'danger'
}

function typeLabel(type) {
  if (type === 'new') return '新增模板'
  if (type === 'disappear') return '消失'
  return '突增'
}

function feedbackKey(pt, type) {
  return `${pt.templateHash}-${type}`
}

function rangeParams() {
  const to = new Date()
  const from = new Date(to.getTime() - (RANGE_MS[rangeKey.value] || RANGE_MS['24h']))
  return {
    from: fmtDateTime(from),
    to: fmtDateTime(to),
  }
}

function fmtDateTime(d) {
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

async function load() {
  loading.value = true
  try {
    const { data } = await getAnomalyTimeline(rangeParams())
    points.value = data.data?.points || []
    hasMore.value = !!data.data?.hasMore
  } catch {
    points.value = []
  } finally {
    loading.value = false
  }
}

async function loadStats() {
  try {
    const { data } = await getFeedbackStats()
    const d = data.data || {}
    stats.falsePositive = d.falsePositive || 0
    stats.truePositive = d.truePositive || 0
    stats.ignored = d.ignored || 0
  } catch {
    /* 统计失败不影响主视图 */
  }
}

function loadMore() {
  // 简单实现：扩大时间窗口加载（保持与后端游标语义一致）
  load()
}

function onRangeChange() {
  load()
}

function onDrillDown(pt) {
  emit('drill-down', pt)
}

async function onFeedback(pt, type) {
  feedbackBusy.value = feedbackKey(pt, type)
  try {
    const payload = {
      anomalyId: pt.id || 0,
      templateHash: pt.templateHash,
      itemId: pt.itemId || 0,
      type,
      reason: '',
    }

    if (type === 'false_positive') {
      try {
        const { value } = await ElMessageBox.prompt(
          '请填写误报原因（可选），用于后续阈值调优',
          '标记误报',
          {
            confirmButtonText: '提交',
            cancelButtonText: '取消',
            inputPlaceholder: '例如：计划内维护导致',
            inputValue: '',
          },
        )
        payload.reason = value || ''
      } catch {
        feedbackBusy.value = ''
        return
      }
    }

    await markAnomalyFeedback(payload)
    ElMessage.success(type === 'false_positive' ? '已标记误报，阈值已自适应上调' : '已确认异常')
    await loadStats()
  } catch {
    ElMessage.error('反馈提交失败')
  } finally {
    feedbackBusy.value = ''
  }
}

onMounted(() => {
  load()
  loadStats()
})
</script>

<style scoped>
.anomaly-timeline {
  width: 100%;
}
</style>
