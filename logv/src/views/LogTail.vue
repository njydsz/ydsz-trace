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
    <el-card shadow="never" class="!rounded-xl">
      <el-form :model="form" label-position="top" class="grid-form">
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
      </el-form>

      <el-form :model="form" label-position="top">
        <el-form-item label="过滤关键词">
          <el-input v-model="form.key" placeholder="留空则显示全部" clearable />
        </el-form-item>
      </el-form>

      <div class="flex flex-wrap items-center gap-3 pb-3">
        <el-checkbox v-model="form.regex">
          <span class="text-sm">正则匹配</span>
        </el-checkbox>
        <el-select v-model="form.level" placeholder="不过滤级别" clearable style="width: 140px">
          <el-option label="DEBUG" value="DEBUG" />
          <el-option label="INFO" value="INFO" />
          <el-option label="WARN" value="WARN" />
          <el-option label="ERROR" value="ERROR" />
          <el-option label="FATAL" value="FATAL" />
        </el-select>
        <div class="flex items-center gap-2">
          <span class="text-sm text-gray-500">模板提取</span>
          <el-switch v-model="form.extractTemplate" active-text="提取" inactive-text="完整" />
        </div>
      </div>

      <div class="flex items-center gap-3">
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
      </div>
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
            <el-tag v-if="rateLimitedCount > 0" type="warning" size="small" class="ml-2">
              速率限制丢弃: {{ rateLimitedCount }} 行
            </el-tag>
          </div>
        </div>
      </template>
      <div class="log-viewer bg-gray-900 rounded p-3 h-[500px] overflow-hidden font-mono text-sm">
        <!-- V2.4-08 虚拟滚动：改用 DynamicScroller 支持可变行高。
             原因：日志开启 pre-wrap 后行高不再固定，固定的 item-size
             会导致滚动位置错乱与白屏。DynamicScroller 会实测每行高度。 -->
        <DynamicScroller
          ref="scrollerRef"
          class="log-scroller"
          :items="displayLines"
          :min-item-size="28"
          key-field="__lineKey"
          v-slot="{ item, index, active }"
        >
          <DynamicScrollerItem
            :item="item"
            :active="active"
            :data-index="index"
            :size-dependencies="[item.content, isMultiNode]"
          >
          <div
            class="log-line hover:bg-gray-800 group relative"
            :class="getLineClass(item)"
          >
            <span class="text-gray-500 mr-2 select-none">{{ sortedIndexStart + index + 1 }}</span>
            <template v-if="isMultiNode">
              <span class="text-blue-400 mr-1 text-xs font-mono">{{ item.ip }}:{{ item.port }}</span>
              <span v-if="item.rateLimited" class="text-yellow-400 mr-1 text-xs">[限速丢弃 {{ item.droppedCount }}]</span>
              <span v-else-if="item.template && form.extractTemplate" class="text-purple-300 mr-1 text-xs">{{ item.template }}</span>
            </template>
            <span class="log-content">{{ item.rateLimited ? item.content : (item.content || item) }}</span>
            <el-button
              :icon="CopyDocument"
              size="small"
              class="copy-hover-btn absolute right-1 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 transition-opacity"
              @click.stop="handleCopy(item)"
            />
          </div>
          </DynamicScrollerItem>
        </DynamicScroller>
        <div v-if="displayLines.length === 0" class="text-gray-500 text-center py-8">
          点击"开始追踪"查看实时日志
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, shallowRef, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { VideoPlay, VideoPause, Delete, CopyDocument, Check } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { DynamicScroller, DynamicScrollerItem } from 'vue-virtual-scroller'
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css'
import { queryLogClients, queryItemsByClient } from '@/api/logs'
import { createSSE } from '@/api/sse'

// 环形缓冲区最大容量
const RING_BUFFER_SIZE = 2000

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const streaming = ref(false)
const paused = ref(false)
const totalCount = ref(0)

// 用 shallowRef 包裹行数组，避免深度响应式开销（SSE 高频推送）
// ringBuffer 存储完整的环形缓冲区，用于视图渲染
const ringBuffer = shallowRef([])
const scrollerRef = ref(null)

const displayLines = computed(() => ringBuffer.value)

const sortedIndexStart = ref(0)

// 当前 SSE 连接实例
let currentSSE = null
let msgCount = 0
const rateLimitedCount = ref(0)

// 是否是 client_id=0 的多节点模式
const isMultiNode = computed(() => form.client === 0)

const form = reactive({
  client: null,
  item: null,
  date: '',
  key: '',
  regex: false,
  level: '',
  extractTemplate: false,
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
      ...(form.extractTemplate ? { extract: 'template' } : {}),
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
      try {
        const parsed = JSON.parse(data)
        msgCount++
        totalCount.value = msgCount
        if (!paused.value) {
          appendToRing(parsed)
        }
        if (parsed.rateLimited && parsed.droppedCount) {
          rateLimitedCount.value += parsed.droppedCount
        }
      } catch {
        // 兼容旧格式（纯字符串行）
        msgCount++
        totalCount.value = msgCount
        if (!paused.value) {
          appendToRing(data)
        }
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
  const lineKey = `${Date.now()}_${msgCount}`
  // 为每行添加唯一 key 供 RecycleScroller 使用
  const lineWithKey = (typeof line === 'object' && line !== null)
    ? { ...line, __lineKey: lineKey }
    : { content: line, __lineKey: lineKey }

  if (buf.length < RING_BUFFER_SIZE) {
    // 未满时直接追加
    ringBuffer.value = [...buf, lineWithKey]
  } else {
    // 满了之后，在新数组中替换最早的元素
    const newArr = buf.slice(1)
    newArr.push(lineWithKey)
    ringBuffer.value = newArr
    sortedIndexStart.value++
  }

  // 滚动到底部
  scrollToBottom()
}

async function scrollToBottom() {
  await nextTick()
  if (scrollerRef.value) {
    scrollerRef.value.scrollTop = scrollerRef.value.scrollHeight
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
  rateLimitedCount.value = 0
}

function getLineClass(line) {
  if (!line) return ''
  const text = typeof line === 'object' ? (line.content || '') : String(line)
  if (line.rateLimited) return 'text-yellow-500 italic'
  const upper = text.toUpperCase()
  if (upper.includes('FATAL')) return 'text-red-400 font-bold'
  if (upper.includes('ERROR')) return 'text-red-400 font-semibold'
  if (upper.includes('WARN')) return 'text-yellow-400'
  if (upper.includes('DEBUG')) return 'text-gray-500'
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
/* 横向铺满表单 */
.grid-form {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 0 20px;
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
}

/* 虚拟滚动日志流 */
.log-viewer {
  display: flex;
  flex-direction: column;
}
.log-scroller {
  flex: 1;
  height: 500px;
}
.log-line {
  display: flex;
  align-items: flex-start;
  /* V2.4-08：去掉固定 height，改由 DynamicScroller 实测行高。
     保留 min-height 作为 min-item-size 的对应值。 */
  min-height: 28px;
  padding: 0 4px;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.5;
  font-size: 12px;
  overflow: hidden;
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
