<template>
  <div>
    <!-- 统一页面头部 -->
    <div class="page-header mb-5">
      <h2 class="page-header__title">
        <span class="page-header__icon" style="background: linear-gradient(135deg, #6366f1, #4f46e5); box-shadow: 0 2px 8px rgba(99, 102, 241, 0.2);">
          <el-icon :size="18"><VideoCamera /></el-icon>
        </span>
        实时日志追踪
      </h2>
      <p class="page-header__subtitle">通过 SSE 流式接收实时日志输出，支持正则过滤与日志级别筛选</p>
    </div>
    <el-card shadow="never" class="max-w-4xl">
      <el-form :model="form" label-width="100px">
        <el-form-item label="客户端">
          <el-select v-model="form.client" placeholder="选择客户端" style="width: 100%">
            <el-option v-for="c in clients" :key="c.id" :label="`${c.ip}:${c.port}`" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="日志项">
          <el-select v-model="form.item" placeholder="选择日志项" style="width: 100%" :loading="itemsLoading">
            <el-option v-for="it in items" :key="it.id" :label="it.itemName" :value="it.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="日期">
          <el-date-picker v-model="form.date" type="date" value-format="YYYYMMDD" placeholder="选择日期" style="width: 100%" />
        </el-form-item>
        <el-form-item label="过滤关键词">
          <el-input v-model="form.key" placeholder="留空则显示全部" clearable />
        </el-form-item>
        <el-form-item label="正则模式">
          <el-checkbox v-model="form.regex">启用正则匹配</el-checkbox>
        </el-form-item>
        <el-form-item label="日志级别">
          <el-select v-model="form.level" placeholder="不过滤" clearable style="width: 100%">
            <el-option label="DEBUG" value="DEBUG" />
            <el-option label="INFO" value="INFO" />
            <el-option label="WARN" value="WARN" />
            <el-option label="ERROR" value="ERROR" />
            <el-option label="FATAL" value="FATAL" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :icon="VideoPlay" :disabled="!canStart" @click="startTail">
            开始追踪
          </el-button>
          <el-button :icon="VideoPause" :disabled="!streaming" @click="stopTail">
            停止追踪
          </el-button>
          <el-button :icon="Delete" @click="clearLog">清空</el-button>
          <el-button :icon="paused ? Check : VideoPause" :disabled="!streaming" @click="togglePause">
            {{ paused ? '恢复' : '暂停接收' }}
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="mt-4">
      <template #header>
        <div class="flex justify-between items-center">
          <span>实时日志流</span>
          <div class="flex items-center gap-2">
            <el-tag :type="streaming ? 'success' : 'info'" size="small">
              {{ streaming ? (paused ? '已暂停' : '追踪中') : '已停止' }}
            </el-tag>
            <span class="text-gray-400 text-sm">显示行数: {{ displayLines.length }} / 总计 {{ totalCount }}</span>
          </div>
        </div>
      </template>
      <div ref="viewerRef" class="log-viewer bg-gray-900 rounded p-3 h-[500px] overflow-y-auto font-mono text-sm">
        <div
          v-for="(line, idx) in displayLines"
          :key="idx"
          class="log-line hover:bg-gray-800 group relative"
          :class="getLineClass(line)"
        >
          <span class="text-gray-500 mr-2 select-none">{{ sortedIndexStart + idx + 1 }}</span>
          <span class="log-content">{{ line }}</span>
          <el-button
            :icon="CopyDocument"
            size="small"
            class="copy-hover-btn absolute right-1 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 transition-opacity"
            @click.stop="handleCopy(line)"
          />
        </div>
        <div v-if="displayLines.length === 0" class="text-gray-500 text-center py-8">
          点击"开始追踪"查看实时日志
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, shallowRef, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { VideoPlay, VideoPause, Delete, CopyDocument, Check } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { queryLogClients } from '@/api/logs'
import { createSSE } from '@/api/sse'

// 环形缓冲区最大容量
const RING_BUFFER_SIZE = 500

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const streaming = ref(false)
const paused = ref(false)
const totalCount = ref(0)
const viewerRef = ref(null)

// 用 shallowRef 包裹行数组，避免深度响应式开销（SSE 高频推送）
// ringBuffer 存储完整的环形缓冲区，用于视图渲染
const ringBuffer = shallowRef([])

const displayLines = computed(() => ringBuffer.value)

const sortedIndexStart = ref(0)

// 当前 SSE 连接实例
let currentSSE = null
let msgCount = 0

const form = reactive({
  client: null,
  item: null,
  date: '',
  key: '',
  regex: false,
  level: '',
})

const canStart = computed(() => form.client && form.item && form.date)

async function loadClients() {
  try {
    const { data } = await queryLogClients()
    clients.value = data.data || []
  } catch {
    ElMessage.error('加载客户端列表失败')
  }
}

async function loadItems() {
  try {
    itemsLoading.value = true
    if (form.client) {
      const { queryItemsByClient } = await import('@/api/logs')
      const { data } = await queryItemsByClient(form.client)
      items.value = data.data || []
    }
  } catch {
    items.value = []
  } finally {
    itemsLoading.value = false
  }
}

function startTail() {
  if (!canStart.value) {
    ElMessage.warning('请选择客户端、日志项和日期')
    return
  }

  stopTail()
  streaming.value = true
  paused.value = false
  ringBuffer.value = []
  msgCount = 0
  totalCount.value = 0
  sortedIndexStart.value = 0

  // 使用安全 SSE 封装（自带 CSRF 防护 + 401 处理 + 自动重连）
  currentSSE = createSSE('/logs/tail', {
    method: 'POST',
    body: {
      client: Number(form.client),
      item: Number(form.item),
      date: form.date,
      key: form.key,
      regex: form.regex,
      level: form.level,
    },
    onMessage: (event, data) => {
      if (!streaming.value) return
      handleEvent({ event, data })
    },
    onError: (err) => {
      if (streaming.value) {
        ElMessage.error('流读取失败: ' + err.message)
      }
      streaming.value = false
    },
    onClose: () => {
      streaming.value = false
    },
    maxRetries: 3,
    retryInterval: 3000,
  })
}

function handleEvent({ event, data }) {
  switch (event) {
    case 'connected':
      ElMessage.success('追踪已连接')
      break
    case 'line':
      msgCount++
      totalCount.value = msgCount
      if (!paused.value) {
        appendToRing(data)
      }
      break
    case 'error':
      ElMessage.error(data)
      stopTail()
      break
    case 'done':
      ElMessage.info('追踪结束')
      streaming.value = false
      break
  }
}

// 环形缓冲区写入 - 使用 splice 避免深度响应
function appendToRing(line) {
  const buf = ringBuffer.value
  if (buf.length < RING_BUFFER_SIZE) {
    // 未满时直接追加
    // 触发 shallowRef 响应式更新
    ringBuffer.value = [...buf, line]
  } else {
    // 满了之后，在新数组中替换最早的元素
    const newArr = buf.slice(1)
    newArr.push(line)
    ringBuffer.value = newArr
    sortedIndexStart.value++
  }
}

function togglePause() {
  paused.value = !paused.value
  if (!paused.value) {
    ElMessage.info('已恢复接收')
  } else {
    ElMessage.info('已暂停接收，继续追踪但不更新视图')
  }
}

function stopTail() {
  streaming.value = false
  paused.value = false
  // 关闭 SSE 连接（sse.js 封装会自动清理 reader 和 abort controller）
  if (currentSSE) {
    currentSSE.close()
    currentSSE = null
  }
}

function clearLog() {
  ringBuffer.value = []
  msgCount = 0
  totalCount.value = 0
  sortedIndexStart.value = 0
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

async function handleCopy(line) {
  try {
    await navigator.clipboard.writeText(line)
    ElMessage.success('已复制')
  } catch {
    ElMessage.error('复制失败')
  }
}

onMounted(() => {
  loadClients()
  applyRouteQuery()
})

async function applyRouteQuery() {
  // 从 URL query 预填表单（由检索页"定位到 Tail"联动传入）
  const query = useRoute().query
  if (!query) return
  if (query.client) {
    form.client = Number(query.client)
    await loadItems()
  }
  if (query.item) form.item = Number(query.item)
  if (query.date) form.date = String(query.date)
  if (query.key) form.key = String(query.key)
  if (query.level) form.level = String(query.level)
  if (query.regex === '1' || query.regex === 'true') form.regex = true
}

onUnmounted(() => {
  stopTail()
})
</script>

<style scoped>
.log-viewer::-webkit-scrollbar {
  width: 6px;
}
.log-viewer::-webkit-scrollbar-thumb {
  background: #555;
  border-radius: 3px;
}
.log-line {
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.5;
  padding: 1px 4px;
}
.log-content {
  display: inline;
}
.copy-hover-btn {
  background-color: #37373d;
  border: none;
  color: #d4d4d4;
  width: 24px;
  height: 24px;
}
.copy-hover-btn:hover {
  background-color: #4a4a50 !important;
  color: #fff !important;
}
</style>
