<template>
  <div>
    <!-- 统一页面头部 -->
    <div class="page-header mb-5">
      <h2 class="page-header__title">
        <span class="page-header__icon" style="background: linear-gradient(135deg, #f59e0b, #d97706); box-shadow: 0 2px 8px rgba(245, 158, 11, 0.2);">
          <el-icon :size="18"><Share /></el-icon>
        </span>
        日志链路追踪
      </h2>
      <p class="page-header__subtitle">根据 TraceId 跨节点聚合调用链日志，支持分组视图与瀑布视图</p>
    </div>
    <el-card shadow="never" class="!rounded-xl">
      <el-form :model="form" label-position="top" class="grid-form">
        <el-form-item label="Trace ID" class="span-2">
          <el-input
            v-model="form.traceId"
            placeholder="输入要追踪的 traceId"
            clearable
          />
        </el-form-item>
        <el-form-item label="日期">
          <el-date-picker
            v-model="form.date"
            type="date"
            value-format="YYYYMMDD"
            placeholder="选择日志日期"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="客户端">
          <el-select
            v-model="form.client"
            placeholder="全部客户端（并发检索）"
            style="width: 100%"
            @change="onClientChange"
          >
            <el-option label="全部客户端" :value="0" />
            <el-option v-for="c in clients" :key="c.id" :label="`${c.ip}:${c.port}`" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="日志项">
          <el-select
            v-model="form.item"
            placeholder="选择日志项"
            style="width: 100%"
            :loading="itemsLoading"
          >
            <el-option v-for="it in items" :key="it.id" :label="it.itemName" :value="it.id" />
          </el-select>
        </el-form-item>
      </el-form>

      <div class="flex flex-wrap items-center gap-3 py-3">
        <el-checkbox v-model="form.regex">
          <span class="text-sm">正则匹配<span class="text-gray-400 ml-1">把 traceId 作为正则表达式</span></span>
        </el-checkbox>
      </div>

      <div class="flex items-center gap-3">
        <el-button type="primary" :icon="Link" :loading="loading" @click="onTrace">
          追踪调用链
        </el-button>
        <el-button :icon="Refresh" @click="resetForm">清空</el-button>
      </div>
    </el-card>

    <el-card v-if="result" shadow="never" class="mt-6">
      <el-tabs v-model="resultViewType">
        <el-tab-pane label="节点分组" name="node">
          <template #header>
            <div class="flex items-center justify-between">
              <div class="text-sm text-gray-600">
                TraceId
                <code class="bg-gray-100 px-1.5 py-0.5 rounded mx-1">{{ result.traceId }}</code>
                共 {{ result.Total }} 行命中 / 涉及 {{ result.Nodes }} 个节点 / 展示 {{ result.Results.length }} 个节点
              </div>
            </div>
          </template>

          <div v-if="result.Results.length === 0" class="text-gray-500 text-center py-8">
            未找到匹配 traceId 的日志行
          </div>

          <div v-for="group in result.Results" :key="group.Node" class="mb-6">
            <div class="flex items-center gap-2 mb-2">
              <el-tag size="small" type="primary">{{ group.Node }}</el-tag>
              <span class="text-sm text-gray-500">{{ group.Count }} 行</span>
            </div>
            <div class="bg-gray-900 rounded p-3 max-h-[400px] overflow-y-auto font-mono text-xs">
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
        </el-tab-pane>

        <el-tab-pane label="瀑布视图" name="waterfall">
          <TraceWaterfallView
            :trace-id="form.traceId.trim()"
            :date="form.date"
            :item="form.item"
            :client="form.client"
            :max-per-node="5000"
          />
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Link, Refresh } from '@element-plus/icons-vue'
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
/* 横向铺满表单 */
.grid-form {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 0 20px;
}
.grid-form .span-2 {
  grid-column: span 2;
}
.grid-form :deep(.el-form-item__label) {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-secondary, #6b7280);
  padding-bottom: 4px;
}
@media (max-width: 768px) {
  .grid-form {
    grid-template-columns: 1fr;
  }
  .grid-form .span-2 {
    grid-column: span 1;
  }
}

.log-line {
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.5;
}
</style>
