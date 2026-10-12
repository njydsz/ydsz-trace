<template>
  <div class="flex flex-col h-full">
    <!-- 对话区 -->
    <div ref="chatContainerRef" class="flex-1 overflow-y-auto px-4 py-6 space-y-4">
      <!-- 欢迎消息 -->
      <ChatMessage
        v-if="messages.length === 0"
        role="assistant"
        type="text"
        content="你好！我是日志检索助手。你可以用自然语言描述你想查什么，例如：\n- 今天有哪些 ERROR？\n- 近 1 小时 WARN 级别日志有没有？\n- 查一下 500 错误"
      />

      <!-- 消息列表 -->
      <ChatMessage
        v-for="msg in messages"
        :key="msg.id"
        :role="msg.role"
        :type="msg.type"
        :content="msg.content"
        :timestamp="msg.timestamp"
        :error-title="msg.errorTitle"
        :error-detail="msg.errorDetail"
      >
        <template #chart>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 mt-2">
            <QueryResultCard
              title="命中行数"
              :count="msg.resultTotal"
              :subtitle="msg.timeRangeDetail"
              :details="msg.truncated ? '结果已达上限，建议缩小范围' : ''"
            />
          </div>
          <!-- 命中行预览 -->
          <div v-if="msg.rows && msg.rows.length > 0" class="mt-3 space-y-1 max-h-48 overflow-y-auto">
            <div
              v-for="(row, i) in msg.rows.slice(0, 10)"
              :key="i"
              class="text-xs font-mono bg-gray-50 dark:bg-gray-800 px-2 py-1.5 rounded text-gray-600 dark:text-gray-300"
            >
              {{ typeof row === 'string' ? row : row.line || row.content || JSON.stringify(row) }}
            </div>
            <p v-if="msg.rows.length > 10" class="text-xs text-gray-400 mt-1">
              仅展示前 10 行，共 {{ msg.rows.length }} 行
            </p>
          </div>
        </template>

        <template #fallback>
          <el-button
            v-if="msg.fallbackQuery"
            size="small"
            text
            :icon="Search"
            class="mt-2"
            @click="handleFallbackSearch(msg.fallbackQuery)"
          >
            尝试关键词检索: "{{ msg.fallbackQuery }}"
          </el-button>
        </template>

        <template #footer>
          <ExplanationCard
            v-if="msg.role === 'assistant' && msg.explanation"
            :user-intent="msg.explanation.userIntent"
            :dsl-conditions="msg.explanation.dslConditions"
            :match-count="msg.explanation.matchCount"
          />
        </template>
      </ChatMessage>

      <!-- 加载指示器 -->
      <div v-if="loading" class="flex justify-start">
        <div class="bg-gray-100 dark:bg-gray-700 rounded-2xl rounded-bl-md px-4 py-3 text-sm text-gray-500">
          <span class="inline-flex gap-1 items-center">
            <span class="animate-pulse">正在分析</span>
            <span class="animate-pulse delay-100">.</span>
            <span class="animate-pulse delay-200">.</span>
            <span class="animate-pulse delay-300">.</span>
          </span>
        </div>
      </div>
    </div>

    <!-- 快捷预设栏 -->
    <div class="flex gap-2 px-4 py-2 border-t border-gray-200 dark:border-gray-700 overflow-x-auto">
      <el-button
        v-for="preset in quickPresets"
        :key="preset.label"
        size="plain"
        round
        :icon="preset.icon"
        @click="handleQuickPreset(preset)"
      >
        {{ preset.label }}
      </el-button>
    </div>

    <!-- 输入区 -->
    <div class="px-4 pb-4 pt-2 border-t border-gray-100 dark:border-gray-800">
      <div class="flex items-end gap-3">
        <el-input
          v-model="inputText"
          type="textarea"
          :rows="1"
          :autosize="{ minRows: 1, maxRows: 5 }"
          placeholder="输入检索问题，Enter 发送，Shift+Enter 换行"
          resize="none"
          @keydown="handleKeyDown"
        />
        <el-button
          type="primary"
          :icon="Promotion"
          :loading="loading"
          :disabled="!inputText.trim()"
          circle
          @click="handleSend"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
/**
 * 对话式工作台 FR-1201 — v2.0+ 默认首页。
 *
 * 布局（flex 纵向 100% 高）：对话区自适应增长 + 底部固定输入框。
 * 前端模拟 AI 流程：自然语言关键词匹配 → searchLogs → 结果卡片渲染。
 */
import { ref, nextTick } from 'vue'
import { Promotion, Search, Clock, Sunny, Calendar } from '@element-plus/icons-vue'
import { searchLogs } from '@/api/logs'
import ChatMessage from '@/components/ChatMessage.vue'
import QueryResultCard from '@/components/QueryResultCard.vue'
import ExplanationCard from '@/components/ExplanationCard.vue'

const inputText = ref('')
const loading = ref(false)
const messages = ref([])
const chatContainerRef = ref(null)

let messageIdCounter = 0
function nextId() {
  return ++messageIdCounter
}

// 快捷预设配置
const quickPresets = [
  { label: '近 1 小时 ERROR', icon: Clock, query: '近 1 小时 ERROR 日志', timeRange: '1h', level: 'ERROR' },
  { label: '今天 WARN', icon: Sunny, query: '今天 WARN 级别日志', timeRange: 'today', level: 'WARN' },
  { label: '近 3 天 500 错误', icon: Calendar, query: '近 3 天 500 错误', timeRange: '3d', key: '500' },
]

// 格式化时间为 HH:mm:ss
function formatTime(date) {
  const h = String(date.getHours()).padStart(2, '0')
  const m = String(date.getMinutes()).padStart(2, '0')
  const s = String(date.getSeconds()).padStart(2, '0')
  return `${h}:${m}:${s}`
}

// 格式化日期为 YYYYMMDD
function formatDate(date) {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}${m}${d}`
}

// 解析自然语言为搜索参数
function parseNaturalLanguage(text) {
  const levelPattern = /\b(ERROR|WARN|INFO|DEBUG|FATAL)\b/i
  const levelMatch = text.match(levelPattern)
  const level = levelMatch ? levelMatch[1].toUpperCase() : ''

  // 时间解析
  let date = formatDate(new Date())
  let startTime = ''
  let endTime = ''
  let timeRangeDetail = ''

  if (text.includes('近 1 小时') || text.includes('近1小时') || text.includes('最近 1 小时')) {
    const now = new Date()
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000)
    startTime = formatTime(oneHourAgo)
    endTime = formatTime(now)
    timeRangeDetail = `${startTime} - ${endTime}`
  } else if (text.includes('今天') || text.includes('今日')) {
    const now = new Date()
    startTime = '00:00:00'
    endTime = formatTime(now)
    timeRangeDetail = `今天 ${startTime} - ${endTime}`
  } else if (text.includes('近 3 天') || text.includes('近3天')) {
    const now = new Date()
    const threeDaysAgo = new Date(now.getTime() - 3 * 24 * 60 * 60 * 1000)
    date = formatDate(threeDaysAgo)
    timeRangeDetail = `${date} - ${formatDate(now)}`
  }

  // 提取关键词：引号内容或去除级别词和时间词后的剩余文本
  let key = ''
  const quotedMatch = text.match(/["'"'"']([^"'"'"']+)["'"'"']/)
  if (quotedMatch) {
    key = quotedMatch[1].trim()
  } else {
    // 去除级别和时间关键词后剩余作为查询词
    const remaining = text
      .replace(/近\s*\d+\s*天/g, '')
      .replace(/近\s*1\s*小时/g, '')
      .replace(/最近\s*\d+\s*(小时|天)/g, '')
      .replace(/今天|今日/g, '')
      .replace(/日志|查[一查]?[下看]?[一一下]?/g, '')
      .replace(levelPattern, '')
      .trim()
    key = remaining
  }

  return { level, date, startTime, endTime, key, timeRangeDetail }
}

// 格式化 DSL 条件为可读字符串
function formatDslConditions(params) {
  const parts = []
  if (params.level) parts.push(`级别=${params.level}`)
  if (params.key) parts.push(`关键词="${params.key}"`)
  if (params.date) parts.push(`日期=${params.date}`)
  if (params.startTime && params.endTime) {
    parts.push(`时间=${params.startTime}~${params.endTime}`)
  }
  return parts.length > 0 ? parts.join(' AND ') : '无条件（全量）'
}

// 快捷预设点击
function handleQuickPreset(preset) {
  inputText.value = preset.query
}

// 处理键盘事件：Enter 发送 / Shift+Enter 换行
function handleKeyDown(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

// 滚动到底部
async function scrollToBottom() {
  await nextTick()
  if (chatContainerRef.value) {
    chatContainerRef.value.scrollTop = chatContainerRef.value.scrollHeight
  }
}

// 添加用户消息
function addUserMessage(text) {
  messages.value.push({
    id: nextId(),
    role: 'user',
    type: 'text',
    content: text,
    timestamp: formatTime(new Date()),
  })
}

// 添加助手消息（带结果卡片）
function addAssistantMessage(data) {
  messages.value.push({
    id: nextId(),
    role: 'assistant',
    type: 'chart',
    content: data.summary || '',
    resultTotal: data.total || 0,
    rows: data.rows || [],
    truncated: data.truncated || false,
    timeRangeDetail: data.timeRangeDetail || '',
    explanation: data.explanation || null,
  })
}

// 添加错误消息
function addErrorMessage(text, fallbackQuery) {
  messages.value.push({
    id: nextId(),
    role: 'assistant',
    type: 'error',
    content: '',
    errorTitle: '查询遇到问题',
    errorDetail: text,
    fallbackQuery: fallbackQuery || '',
  })
}

// 执行日志搜索
async function executeSearch(queryText) {
  const params = parseNaturalLanguage(queryText)
  const dslConditions = formatDslConditions(params)

  loading.value = true
  try {
    const res = await searchLogs({
      item: 1,
      date: params.date,
      key: params.key,
      level: params.level,
      startTime: params.startTime,
      endTime: params.endTime,
      pageNo: 1,
      pageSize: 50,
    })

    const payload = res.data || {}
    if (payload.code && payload.code !== '200') {
      addErrorMessage(
        payload.msg || '检索失败，请尝试调整关键词',
        params.key || params.level || '',
      )
      return
    }

    const data = payload.data || {}
    const total = data.total || 0
    const rows = data.list || []

    if (total === 0) {
      addAssistantMessage({
        summary: `未找到匹配「${queryText}」的日志记录。建议调整关键词或扩大时间范围。`,
        total: 0,
        rows: [],
        truncated: false,
        timeRangeDetail: params.timeRangeDetail,
        explanation: {
          userIntent: queryText,
          dslConditions,
          matchCount: 0,
        },
      })
    } else {
      addAssistantMessage({
        summary: `找到 ${total} 条匹配记录。以下是检索结果概览：`,
        total,
        rows,
        truncated: !!data.truncated,
        timeRangeDetail: params.timeRangeDetail,
        explanation: {
          userIntent: queryText,
          dslConditions,
          matchCount: total,
        },
      })
    }
  } catch {
    addErrorMessage(
      '无法连接检索服务，请确认服务端已启动或网络可达。',
      params.key || params.level || '',
    )
  } finally {
    loading.value = false
  }
}

// 降级搜索（用关键词重试）
function handleFallbackSearch(keyword) {
  if (keyword) {
    inputText.value = keyword
    handleSend()
  }
}

// 发送消息
async function handleSend() {
  const text = inputText.value.trim()
  if (!text || loading.value) return

  addUserMessage(text)
  inputText.value = ''
  await scrollToBottom()
  await executeSearch(text)
  await scrollToBottom()
}

// 导出给测试使用
defineExpose({
  messages,
  inputText,
  parseNaturalLanguage,
  handleSend,
  handleQuickPreset,
})
</script>
