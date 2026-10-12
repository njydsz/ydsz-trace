<template>
  <div class="flex flex-col h-full chat-layout">
    <!-- 对话区 -->
    <div ref="chatContainerRef" class="flex-1 overflow-y-auto chat-scroll">
      <!-- 欢迎界面 -->
      <div v-if="messages.length === 0" class="welcome-screen h-full flex flex-col items-center justify-center">
        <div class="welcome-content text-center">
          <div class="welcome-icon flex items-center justify-center rounded-2xl mx-auto mb-5">
            <el-icon :size="36" class="text-white"><ChatLineRound /></el-icon>
          </div>
          <h2 class="welcome-title">你好！我是日志检索助手</h2>
          <p class="welcome-desc mt-2">你可以用自然语言描述你想查什么</p>
          <div class="welcome-examples mt-5 flex flex-wrap justify-center gap-2">
            <span class="example-tag">"今天有哪些 ERROR？"</span>
            <span class="example-tag">"近 1 小时 WARN 日志"</span>
            <span class="example-tag">"查一下 500 错误"</span>
          </div>
        </div>
      </div>

      <!-- 消息列表 -->
      <div v-else class="chat-messages px-4 py-6 space-y-5">
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
            <div v-if="msg.rows && msg.rows.length > 0" class="mt-3 space-y-1 max-h-48 overflow-y-auto rounded-lg bg-muted-bg p-2">
              <div
                v-for="(row, i) in msg.rows.slice(0, 10)"
                :key="i"
                class="text-xs font-mono text-secondary px-2 py-1.5 rounded"
              >
                {{ typeof row === 'string' ? row : row.line || row.content || JSON.stringify(row) }}
              </div>
              <p v-if="msg.rows.length > 10" class="text-xs text-muted mt-1 pl-2">
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
      </div>

      <!-- 加载指示器 -->
      <div v-if="loading" class="px-4 pb-4 flex justify-start">
        <div class="typing-indicator flex items-center gap-1.5 bg-card rounded-2xl rounded-bl-md px-4 py-3 shadow-card">
          <span class="dot dot-1" />
          <span class="dot dot-2" />
          <span class="dot dot-3" />
          <span class="typing-text ml-2">正在分析</span>
        </div>
      </div>
    </div>

    <!-- 快捷预设栏 -->
    <div class="chat-presets flex gap-2 px-4 py-2.5 border-t border-light overflow-x-auto">
      <button
        v-for="preset in quickPresets"
        :key="preset.label"
        class="preset-btn flex items-center gap-1.5 px-3.5 py-1.5 rounded-full text-sm font-medium whitespace-nowrap"
        :icon="preset.icon"
        @click="handleQuickPreset(preset)"
      >
        <el-icon :size="14"><component :is="preset.icon" /></el-icon>
        {{ preset.label }}
      </button>
    </div>

    <!-- 输入区 -->
    <div class="chat-input-area px-4 pb-4 pt-2 border-t border-light">
      <div class="input-wrapper flex items-end gap-3 rounded-2xl p-3">
        <el-input
          v-model="inputText"
          type="textarea"
          :rows="1"
          :autosize="{ minRows: 1, maxRows: 5 }"
          placeholder="输入检索问题，Enter 发送，Shift+Enter 换行"
          resize="none"
          class="chat-input"
          @keydown="handleKeyDown"
        />
        <el-button
          type="primary"
          :icon="Promotion"
          :loading="loading"
          :disabled="!inputText.trim()"
          circle
          class="send-btn flex-shrink-0"
          @click="handleSend"
        />
      </div>
      <p class="chat-input-tip text-xs text-muted mt-1.5 text-center">
        支持自然语言 · 试试 "近 1 小时 ERROR 日志"
      </p>
    </div>
  </div>
</template>

<script setup>
/**
 * 对话式工作台 FR-1201 — v2.0+ 默认首页。增强版 UI。
 *
 * 特性: 欢迎界面、快捷预设栏(胶囊按钮)、动画输入框、打字指示器。
 */
import { ref, nextTick } from 'vue'
import { Promotion, Search, Clock, Sunny, Calendar, ChatLineRound } from '@element-plus/icons-vue'
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

const quickPresets = [
  { label: '近 1 小时 ERROR', icon: Clock, query: '近 1 小时 ERROR 日志', timeRange: '1h', level: 'ERROR' },
  { label: '今天 WARN', icon: Sunny, query: '今天 WARN 级别日志', timeRange: 'today', level: 'WARN' },
  { label: '近 3 天 500 错误', icon: Calendar, query: '近 3 天 500 错误', timeRange: '3d', key: '500' },
]

function formatTime(date) {
  const h = String(date.getHours()).padStart(2, '0')
  const m = String(date.getMinutes()).padStart(2, '0')
  const s = String(date.getSeconds()).padStart(2, '0')
  return `${h}:${m}:${s}`
}

function formatDate(date) {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}${m}${d}`
}

function parseNaturalLanguage(text) {
  const levelPattern = /\b(ERROR|WARN|INFO|DEBUG|FATAL)\b/i
  const levelMatch = text.match(levelPattern)
  const level = levelMatch ? levelMatch[1].toUpperCase() : ''
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
  let key = ''
  const quotedMatch = text.match(/["'"'"']([^"'"'"']+)["'"'"']/)
  if (quotedMatch) {
    key = quotedMatch[1].trim()
  } else {
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

function formatDslConditions(params) {
  const parts = []
  if (params.level) parts.push(`级别=${params.level}`)
  if (params.key) parts.push(`关键词="${params.key}"`)
  if (params.date) parts.push(`日期=${params.date}`)
  if (params.startTime && params.endTime) parts.push(`时间=${params.startTime}~${params.endTime}`)
  return parts.length > 0 ? parts.join(' AND ') : '无条件（全量）'
}

function handleQuickPreset(preset) {
  inputText.value = preset.query
}

function handleKeyDown(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

async function scrollToBottom() {
  await nextTick()
  if (chatContainerRef.value) {
    chatContainerRef.value.scrollTop = chatContainerRef.value.scrollHeight
  }
}

function addUserMessage(text) {
  messages.value.push({ id: nextId(), role: 'user', type: 'text', content: text, timestamp: formatTime(new Date()) })
}

function addAssistantMessage(data) {
  messages.value.push({
    id: nextId(), role: 'assistant', type: 'chart', content: data.summary || '',
    resultTotal: data.total || 0, rows: data.rows || [], truncated: data.truncated || false,
    timeRangeDetail: data.timeRangeDetail || '', explanation: data.explanation || null,
  })
}

function addErrorMessage(text, fallbackQuery) {
  messages.value.push({
    id: nextId(), role: 'assistant', type: 'error', content: '',
    errorTitle: '查询遇到问题', errorDetail: text, fallbackQuery: fallbackQuery || '',
  })
}

async function executeSearch(queryText) {
  const params = parseNaturalLanguage(queryText)
  const dslConditions = formatDslConditions(params)
  loading.value = true
  try {
    const res = await searchLogs({
      item: 1, date: params.date, key: params.key, level: params.level,
      startTime: params.startTime, endTime: params.endTime,
      pageNo: 1, pageSize: 50,
    })
    const payload = res.data || {}
    if (payload.code && payload.code !== '200') {
      addErrorMessage(payload.msg || '检索失败，请尝试调整关键词', params.key || params.level || '')
      return
    }
    const data = payload.data || {}
    const total = data.total || 0
    const rows = data.list || []
    if (total === 0) {
      addAssistantMessage({
        summary: `未找到匹配「${queryText}」的日志记录。建议调整关键词或扩大时间范围。`,
        total: 0, rows: [], truncated: false, timeRangeDetail: params.timeRangeDetail,
        explanation: { userIntent: queryText, dslConditions, matchCount: 0 },
      })
    } else {
      addAssistantMessage({
        summary: `找到 ${total} 条匹配记录。以下是检索结果概览：`,
        total, rows, truncated: !!data.truncated, timeRangeDetail: params.timeRangeDetail,
        explanation: { userIntent: queryText, dslConditions, matchCount: total },
      })
    }
  } catch {
    addErrorMessage('无法连接检索服务，请确认服务端已启动或网络可达。', params.key || params.level || '')
  } finally {
    loading.value = false
  }
}

function handleFallbackSearch(keyword) {
  if (keyword) { inputText.value = keyword; handleSend() }
}

async function handleSend() {
  const text = inputText.value.trim()
  if (!text || loading.value) return
  addUserMessage(text)
  inputText.value = ''
  await scrollToBottom()
  await executeSearch(text)
  await scrollToBottom()
}

defineExpose({ messages, inputText, parseNaturalLanguage, handleSend, handleQuickPreset })
</script>

<style scoped>
/* ===== 布局 ===== */
.chat-layout {
  height: calc(100vh - var(--header-height) - 40px);
  min-height: 500px;
}
.chat-scroll {
  scroll-behavior: smooth;
}

/* ===== 欢迎界面 ===== */
.welcome-screen {
  padding: 0 20px;
}
.welcome-icon {
  width: 64px;
  height: 64px;
  background: linear-gradient(135deg, var(--brand-500), var(--brand-700));
  box-shadow: 0 8px 24px rgba(37, 99, 235, 0.25);
}
.welcome-title {
  font-size: 22px;
  font-weight: 700;
  color: var(--text-primary);
  margin: 0;
}
.welcome-desc {
  font-size: 14px;
  color: var(--text-secondary);
  margin: 0;
}
.welcome-examples {
  max-width: 500px;
}
.example-tag {
  display: inline-block;
  padding: 6px 14px;
  font-size: 13px;
  color: var(--text-secondary);
  background: var(--bg-card-muted);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-full);
  transition: all var(--transition-fast);
}
.example-tag:hover {
  background: var(--brand-50);
  border-color: var(--brand-200);
  color: var(--brand-700);
}

/* ===== 消息列表 ===== */
.chat-messages {
  max-width: 800px;
  margin: 0 auto;
}
.bg-muted-bg {
  background: var(--bg-card-muted);
}

/* ===== 打字指示器 ===== */
.typing-indicator {
  border: 1px solid var(--border-light);
}
.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
}
.dot-1 { animation: bounce 1.2s infinite 0s; }
.dot-2 { animation: bounce 1.2s infinite 0.2s; }
.dot-3 { animation: bounce 1.2s infinite 0.4s; }
.typing-text {
  font-size: 13px;
  color: var(--text-muted);
}
@keyframes bounce {
  0%, 60%, 100% { transform: translateY(0); opacity: 0.5; }
  30% { transform: translateY(-4px); opacity: 1; }
}

/* ===== 快捷预设 ===== */
.chat-presets {
  background: transparent;
}
.preset-btn {
  background: var(--bg-card);
  border: 1px solid var(--border-light);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
  outline: none;
  font-family: inherit;
}
.preset-btn:hover {
  background: var(--brand-50);
  border-color: var(--brand-300);
  color: var(--brand-700);
}

/* ===== 输入区 ===== */
.chat-input-area {
  background: transparent;
}
.input-wrapper {
  background: var(--bg-card);
  border: 1px solid var(--border-light);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}
.input-wrapper:focus-within {
  border-color: var(--brand-400);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.08);
}
.chat-input :deep(.el-textarea__inner) {
  border: none !important;
  box-shadow: none !important;
  background: transparent !important;
  padding: 4px 0 !important;
  font-size: 14px;
  line-height: 1.5;
}
.chat-input :deep(.el-textarea__inner:focus) {
  box-shadow: none !important;
}
.send-btn {
  width: 38px;
  height: 38px;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.25);
}
.chat-input-tip {
  text-align: center;
}
</style>
