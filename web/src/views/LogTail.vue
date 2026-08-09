<template>
  <div>
    <h2 class="text-xl font-semibold mb-4">实时日志追踪</h2>
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
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="mt-4">
      <template #header>
        <div class="flex justify-between items-center">
          <span>实时日志流</span>
          <div class="flex items-center gap-2">
            <el-tag :type="streaming ? 'success' : 'info'" size="small">
              {{ streaming ? '追踪中' : '已停止' }}
            </el-tag>
            <span class="text-gray-400 text-sm">接收行数: {{ lines.length }}</span>
          </div>
        </div>
      </template>
      <div class="log-viewer bg-gray-900 rounded p-3 h-[500px] overflow-y-auto font-mono text-sm">
        <div
          v-for="(line, idx) in lines"
          :key="idx"
          class="log-line"
          :class="getLineClass(line)"
        >
          <span class="text-gray-500 mr-2">{{ idx + 1 }}</span>
          <span>{{ line }}</span>
        </div>
        <div v-if="lines.length === 0" class="text-gray-500 text-center py-8">
          点击"开始追踪"查看实时日志
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { VideoPlay, VideoPause, Delete } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { queryLogClients } from '@/api/logs'

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const streaming = ref(false)
const lines = ref([])

// 持有当前 SSE 连接的 reader 与 abortCtrl，用于停止时彻底关闭流
let currentReader = null
let currentAbort = null
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
    // 简化：直接从客户端列表加载
    itemsLoading.value = true
    // 实际使用时需要调用 item API
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
  lines.value = []
  msgCount = 0

  // AbortController 用于在停止/卸载时主动断开 fetch 连接
  const abort = new AbortController()
  currentAbort = abort

  // 使用 fetch + ReadableStream 实现 SSE（后端路由: POST /logs/tail）
  fetch('/logs/tail', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
    body: JSON.stringify({
      client: Number(form.client),
      item: Number(form.item),
      date: form.date,
      key: form.key,
      regex: form.regex,
      level: form.level,
    }),
    signal: abort.signal,
  }).then(response => {
    if (!response.ok) {
      ElMessage.error('连接失败: HTTP ' + response.status)
      streaming.value = false
      return
    }
    const reader = response.body.getReader()
    currentReader = reader
    const decoder = new TextDecoder()
    let buffer = ''

    function readChunk() {
      reader.read().then(({ done, value }) => {
        if (done) {
          streaming.value = false
          return
        }
        if (!streaming.value) {
          return
        }
        buffer += decoder.decode(value, { stream: true })
        const parts = buffer.split('\n\n')
        buffer = parts.pop()
        for (const part of parts) {
          const event = parseSSEEvent(part)
          if (event) handleEvent(event)
        }
        readChunk()
      }).catch(err => {
        if (err.name === 'AbortError') {
          // 用户主动停止，无需提示
          return
        }
        if (streaming.value) {
          ElMessage.error('流读取失败: ' + err.message)
        }
        streaming.value = false
      })
    }
    readChunk()
  }).catch(err => {
    if (err.name === 'AbortError') {
      return
    }
    ElMessage.error('连接失败: ' + err.message)
    streaming.value = false
  })
}

function parseSSEEvent(block) {
  let event = 'message'
  let data = ''
  for (const line of block.split('\n')) {
    if (line.startsWith('event: ')) {
      event = line.slice(7).trim()
    } else if (line.startsWith('data: ')) {
      data += (data ? '\n' : '') + line.slice(6)
    }
  }
  if (!data) return null
  return { event, data }
}

function handleEvent({ event, data }) {
  switch (event) {
    case 'connected':
      ElMessage.success('追踪已连接')
      break
    case 'line':
      msgCount++
      if (lines.value.length > 5000) {
        lines.value.splice(0, 1000)
      }
      lines.value.push(data)
      // 自动滚动
      setTimeout(() => {
        const viewer = document.querySelector('.log-viewer')
        if (viewer) viewer.scrollTop = viewer.scrollHeight
      }, 10)
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

function stopTail() {
  streaming.value = false
  // 主动中止 fetch 连接 + 取消 reader，避免流资源泄漏
  if (currentReader) {
    currentReader.cancel().catch(() => {})
    currentReader = null
  }
  if (currentAbort) {
    currentAbort.abort()
    currentAbort = null
  }
}

function clearLog() {
  lines.value = []
  msgCount = 0
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
    // 需要加载对应客户端的日志项列表，否则选择器无法显示名称
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
}
</style>
