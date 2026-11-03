<template>
  <div class="dashboard-insight space-y-4">
    <!-- 顶部摘要卡片 -->
    <div class="grid grid-cols-4 gap-3">
      <div class="insight-card">
        <div class="text-xs text-muted">在线客户端</div>
        <div class="text-xl font-bold text-success">
          {{ health.onlineClients }}/{{ health.totalClients }}
        </div>
      </div>
      <div class="insight-card">
        <div class="text-xs text-muted">监控日志项</div>
        <div class="text-xl font-bold text-primary">{{ health.totalItems }}</div>
      </div>
      <div class="insight-card">
        <div class="text-xs text-muted">近期任务</div>
        <div class="text-xl font-bold">
          <span class="text-success">{{ health.tasksRecent.success }}</span>
          <span class="text-muted text-sm"> / {{ health.tasksRecent.total }}</span>
        </div>
      </div>
      <div class="insight-card">
        <div class="text-xs text-muted">失败任务</div>
        <div class="text-xl font-bold" :class="health.tasksRecent.failed > 0 ? 'text-danger' : 'text-muted'">
          {{ health.tasksRecent.failed }}
        </div>
      </div>
    </div>

    <!-- 模板趋势 + 异常时间线 -->
    <div class="grid grid-cols-2 gap-4">
      <!-- 模板趋势 -->
      <div class="panel-card">
        <h3 class="panel-title">🔥 Top 模板趋势</h3>
        <div v-if="loading" class="flex justify-center py-6">
          <el-icon class="animate-spin text-2xl text-primary-500"><Loading /></el-icon>
        </div>
        <div v-else-if="trends.length === 0" class="text-center text-muted text-sm py-6">
          暂无模板数据
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="(item, idx) in trends"
            :key="item.hash"
            class="trend-item"
          >
            <span class="trend-rank">{{ idx + 1 }}</span>
            <span
              class="trend-level-tag"
              :class="getLevelTagClass(item.level)"
            >{{ item.level || '-' }}</span>
            <div class="trend-content">
              <div class="text-xs text-gray-700 truncate" :title="item.sample">
                {{ item.sample }}
              </div>
              <div class="text-[10px] text-muted mt-0.5">
                {{ item.lastTime }} · 命中 {{ item.count.toLocaleString() }} 次
              </div>
            </div>
            <span class="trend-count">{{ item.count.toLocaleString() }}</span>
          </div>
        </div>
      </div>

      <!-- 异常事件时间线 -->
      <div class="panel-card">
        <h3 class="panel-title">⚠️ 异常事件时间线（{{ rangeLabel }}）</h3>
        <div v-if="loading" class="flex justify-center py-6">
          <el-icon class="animate-spin text-2xl text-primary-500"><Loading /></el-icon>
        </div>
        <div v-else-if="timeline.length === 0" class="text-center text-muted text-sm py-6">
          {{ rangeLabel }}内无异常事件
        </div>
        <div v-else class="relative pl-4">
          <div class="absolute left-[7px] top-2 bottom-2 w-px bg-gray-200"></div>
          <div
            v-for="point in timeline"
            :key="point.timeBucket"
            class="timeline-point"
          >
            <div class="timeline-dot" :class="point.count > 5 ? 'bg-red-500' : 'bg-orange-400'"></div>
            <div class="timeline-time text-xs font-mono">{{ point.timeBucket }}</div>
            <div class="timeline-count text-xs">
              <span class="text-danger font-semibold">{{ point.count }}</span> 次异常
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- V2.2-06 异常概览 + 反馈自适应状态 -->
    <div class="grid grid-cols-2 gap-4">
      <div class="panel-card">
        <h3 class="panel-title">📊 异常概览</h3>
        <div class="flex items-center gap-4 mb-3">
          <div class="text-2xl font-bold" :class="anomaly.total > 0 ? 'text-danger' : 'text-success'">
            {{ anomaly.total }}
          </div>
          <div class="flex flex-wrap gap-2">
            <el-tag
              v-for="(count, type) in anomaly.byType"
              :key="type"
              size="small"
              :type="anomalyTagType(type)"
            >
              {{ anomalyTypeLabel(type) }} {{ count }}
            </el-tag>
            <span v-if="!anomaly.byType || !Object.keys(anomaly.byType).length" class="text-xs text-muted">
              无异常
            </span>
          </div>
        </div>
        <div v-if="anomaly.recent && anomaly.recent.length" class="space-y-1.5">
          <div
            v-for="item in anomaly.recent"
            :key="item.id"
            class="flex items-start gap-2 text-xs"
          >
            <span class="text-muted font-mono shrink-0">{{ formatBriefTime(item.time) }}</span>
            <span class="truncate" :title="item.description">{{ item.description }}</span>
          </div>
        </div>
      </div>

      <div class="panel-card">
        <h3 class="panel-title">🎯 阈值自适应</h3>
        <p class="text-xs text-muted mb-3">
          用户标记的误报会自动抬高对应模板的检测阈值，减少同类噪声。
        </p>
        <div class="grid grid-cols-3 gap-2 text-center">
          <div class="rounded bg-amber-50 dark:bg-amber-900/20 py-2">
            <div class="text-lg font-bold text-amber-600">{{ feedback.falsePositive }}</div>
            <div class="text-[10px] text-muted">误报标记</div>
          </div>
          <div class="rounded bg-green-50 dark:bg-green-900/20 py-2">
            <div class="text-lg font-bold text-green-600">{{ feedback.truePositive }}</div>
            <div class="text-[10px] text-muted">确认异常</div>
          </div>
          <div class="rounded bg-gray-50 dark:bg-gray-800 py-2">
            <div class="text-lg font-bold text-gray-500">{{ feedback.ignored }}</div>
            <div class="text-[10px] text-muted">已忽略</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 服务健康度总览 -->
    <div class="panel-card">
      <h3 class="panel-title">🖥️ 服务健康度总览</h3>
      <div v-if="health.clientHealth.length === 0" class="text-center text-muted text-sm py-4">
        暂无客户端数据
      </div>
      <div v-else class="flex flex-wrap gap-2 mt-2">
        <div
          v-for="client in health.clientHealth"
          :key="client.id"
          class="client-badge"
          :class="client.online ? 'online' : 'offline'"
          :title="client.label"
        >
          <span class="indicator"></span>
          <span class="label">{{ client.label }}</span>
        </div>
      </div>
    </div>

    <!-- 时间范围切换 -->
    <div class="flex items-center gap-2 justify-end pt-2">
      <span class="text-xs text-muted">时间范围：</span>
      <el-radio-group v-model="timeRange" size="small" @change="loadInsight">
        <el-radio-button label="1h">1小时</el-radio-button>
        <el-radio-button label="6h">6小时</el-radio-button>
        <el-radio-button label="24h">24小时</el-radio-button>
        <el-radio-button label="7d">7天</el-radio-button>
      </el-radio-group>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { dashboardInsight } from '@/api/logs'

const emit = defineEmits(['loaded'])

const loading = ref(false)
const timeRange = ref('24h')
const trends = ref([])
const timeline = ref([])
const health = ref({
  totalClients: 0,
  onlineClients: 0,
  totalItems: 0,
  tasksRecent: { total: 0, success: 0, failed: 0 },
  clientHealth: [],
})
const anomaly = ref({ total: 0, byType: {}, recent: [] })
const feedback = ref({ falsePositive: 0, truePositive: 0, ignored: 0 })

const RANGE_LABELS = { '1h': '近 1 小时', '6h': '近 6 小时', '24h': '近 24 小时', '7d': '近 7 天' }

const rangeLabel = computed(() => RANGE_LABELS[timeRange.value] || '近 24 小时')

function formatBriefTime(t) {
  return t ? String(t).slice(5, 16) : '-'
}

function anomalyTypeLabel(type) {
  if (type === 'new') return '新增'
  if (type === 'disappear') return '消失'
  if (type === 'spike') return '突增'
  return type || '-'
}

function anomalyTagType(type) {
  if (type === 'new') return 'info'
  if (type === 'disappear') return 'warning'
  return 'danger'
}

async function loadInsight() {
  loading.value = true
  try {
    const { data: resp } = await dashboardInsight({ timeRange: timeRange.value })
    if (resp.code === 0 && resp.data) {
      trends.value = resp.data.templateTrends || []
      timeline.value = resp.data.errorTimeline || []
      health.value = resp.data.serviceHealth || {
        totalClients: 0, onlineClients: 0, totalItems: 0,
        tasksRecent: { total: 0, success: 0, failed: 0 },
        clientHealth: [],
      }
      // V2.2-06 异常概览与反馈统计（后端可能未实现时降级为空）
      anomaly.value = resp.data.anomalyOverview || { total: 0, byType: {}, recent: [] }
      feedback.value = resp.data.feedbackStats || { falsePositive: 0, truePositive: 0, ignored: 0 }
    }
  } catch (err) {
    console.error('DashboardInsight 加载失败:', err)
  } finally {
    loading.value = false
    emit('loaded')
  }
}

function getLevelTagClass(level) {
  switch (level) {
    case 'ERROR':
    case 'FATAL':
      return 'tag-danger'
    case 'WARN':
      return 'tag-warning'
    case 'INFO':
      return 'tag-success'
    default:
      return 'tag-default'
  }
}

onMounted(() => {
  loadInsight()
})
</script>

<style scoped>
.dashboard-insight {
  padding: 4px;
}

.insight-card {
  background: #fff;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  padding: 12px 16px;
  text-align: center;
}

.panel-card {
  background: #fff;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  padding: 14px 16px;
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  color: #374151;
  margin: 0 0 10px 0;
}

.trend-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  transition: background 0.2s;
}
.trend-item:hover {
  background: #f9fafb;
}

.trend-rank {
  width: 18px;
  height: 18px;
  border-radius: 4px;
  background: #e5e7eb;
  color: #6b7280;
  font-size: 11px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.trend-item:nth-child(1) .trend-rank { background: #fef3c7; color: #d97706; }
.trend-item:nth-child(2) .trend-rank { background: #e0e7ff; color: #6366f1; }
.trend-item:nth-child(3) .trend-rank { background: #fce7f3; color: #db2777; }

.trend-level-tag {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  flex-shrink: 0;
  font-weight: 500;
}
.tag-danger { background: #fef2f2; color: #dc2626; }
.tag-warning { background: #fffbeb; color: #d97706; }
.tag-success { background: #ecfdf5; color: #059669; }
.tag-default { background: #f3f4f6; color: #6b7280; }

.trend-content {
  flex: 1;
  min-width: 0;
}

.trend-count {
  font-size: 12px;
  font-weight: 600;
  color: #6366f1;
  flex-shrink: 0;
}

.timeline-point {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 0;
  position: relative;
}

.timeline-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid #fff;
  flex-shrink: 0;
}

.timeline-time {
  color: #6b7280;
  min-width: 40px;
}

.timeline-count {
  color: #6b7280;
}

.client-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 20px;
  font-size: 11px;
  border: 1px solid #e5e7eb;
  max-width: 180px;
}

.client-badge.online { background: #ecfdf5; border-color: #a7f3d0; }
.client-badge.offline { background: #fef2f2; border-color: #fecaca; }

.client-badge .indicator {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}
.client-badge.online .indicator { background: #10b981; }
.client-badge.offline .indicator { background: #ef4444; }

.client-badge .label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #374151;
  font-family: monospace;
}
</style>
