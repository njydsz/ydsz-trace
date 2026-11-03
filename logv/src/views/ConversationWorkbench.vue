<template>
  <div class="flex h-full chat-layout">
    <!-- 会话列表侧边栏（非移动端显示） -->
    <aside v-if="!isMobile" class="conversation-sidebar flex-shrink-0" data-testid="conversation-sidebar">
      <div class="sidebar-header">
        <el-button type="primary" size="small" :icon="Plus" class="w-full" data-testid="conversation-new-btn" @click="handleNewConversation">
          新会话
        </el-button>
      </div>
      <div class="sidebar-list overflow-y-auto" data-testid="conversation-list">
        <div
          v-for="conv in conversations"
          :key="conv.convId"
          class="conversation-item"
          :class="{ active: conv.convId === currentConvId }"
          data-testid="conversation-item"
          @click="switchConversation(conv.convId)"
        >
          <div class="conv-title">{{ conv.title || '未命名会话' }}</div>
          <div class="conv-meta">
            <span>{{ conv.roleCount || 0 }} 轮</span>
            <span class="conv-time">{{ formatConvTime(conv.updatedTime) }}</span>
          </div>
        </div>
        <div v-if="conversations.length === 0 && !convLoading" class="text-center text-xs text-muted py-8">
          暂无历史会话
        </div>
      </div>
    </aside>

    <!-- 对话主区 -->
    <div class="flex flex-col flex-1 min-w-0">
      <!-- 对话区 -->
      <div
        ref="chatContainerRef"
        :class="[
          'flex-1 overflow-y-auto chat-scroll',
          isMobile ? 'pb-safe-bottom' : ''
        ]"
      >
        <!-- 欢迎界面 -->
        <div v-if="messages.length === 0" class="welcome-screen h-full flex flex-col items-center justify-center">
          <div class="welcome-content text-center" data-testid="conversation-welcome">
            <div class="welcome-icon flex items-center justify-center rounded-2xl mx-auto mb-5">
              <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z"/>
              </svg>
            </div>
            <h2 class="welcome-title" data-testid="conversation-welcome-title">你好！我是日志检索助手</h2>
            <p class="welcome-desc mt-2" data-testid="conversation-welcome-desc">你可以用自然语言描述你想查什么</p>
            <div class="welcome-examples mt-5 flex flex-wrap justify-center gap-2">
              <button class="example-tag cursor-pointer" @click="handleExampleClick('今天有哪些 ERROR？')">"今天有哪些 ERROR？"</button>
              <button class="example-tag cursor-pointer" @click="handleExampleClick('近 1 小时 WARN 日志')">"近 1 小时 WARN 日志"</button>
              <button class="example-tag cursor-pointer" @click="handleExampleClick('查一下 500 错误')">"查一下 500 错误"</button>
            </div>
          </div>
        </div>

        <!-- 消息列表 -->
        <div v-else :class="['px-4 py-6 space-y-5', isMobile ? '' : 'max-w-4xl mx-auto']">
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
              <button
                v-if="msg.fallbackQuery"
                class="mt-2 inline-flex items-center gap-1 text-sm text-brand-600 hover:text-brand-700 font-medium transition-colors"
                @click="handleFallbackSearch(msg.fallbackQuery)"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
                </svg>
                尝试关键词检索: "{{ msg.fallbackQuery }}"
              </button>
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

        <!-- 根因分析三段式结果 -->
        <div v-if="reasonResult" class="px-4 pb-6 max-w-4xl mx-auto">
          <div class="rounded-2xl border border-[var(--border-light)] bg-[var(--bg-card)] p-5 shadow-sm">
            <div class="flex items-center gap-2 mb-4">
              <span class="text-sm font-semibold text-[var(--text-primary)]">根因分析结果</span>
              <el-tag v-if="reasonResult.degraded" size="small" type="warning">降级模式</el-tag>
              <el-tag v-else size="small" type="success">{{ reasonResult.modelName || 'LLM' }}</el-tag>
              <span class="text-xs text-muted ml-auto">共 {{ reasonResult.steps }} 步</span>
            </div>

            <!-- 假设 -->
            <div class="mb-4">
              <h4 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide mb-2">一、根因假设</h4>
              <div v-for="h in (reasonResult.hypotheses || [])" :key="h.index" class="flex gap-2 mb-2">
                <span class="flex-shrink-0 w-5 h-5 rounded-full bg-[var(--brand-100)] text-[var(--brand-700)] text-xs flex items-center justify-center font-medium">{{ h.index }}</span>
                <div class="flex-1">
                  <p class="text-sm text-[var(--text-primary)] leading-relaxed">{{ h.statement }}</p>
                  <span class="text-xs text-muted">置信度 {{ (h.confidence * 100).toFixed(0) }}%</span>
                </div>
              </div>
            </div>

            <!-- 证据 -->
            <div v-if="(reasonResult.evidence || []).length > 0" class="mb-4">
              <h4 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide mb-2">二、验证证据</h4>
              <div class="space-y-2 max-h-64 overflow-y-auto">
                <div v-for="(e, i) in (reasonResult.evidence || [])" :key="i" class="rounded-lg bg-[var(--bg-card-muted)] p-3">
                  <div class="flex items-center gap-2 mb-1">
                    <code class="text-xs font-mono text-[var(--brand-700)]">{{ e.templateHash }}</code>
                    <span class="text-xs text-muted">命中 {{ e.count }} 次</span>
                    <span v-if="e.lineNo" class="text-xs text-muted">行号 {{ e.lineNo }}</span>
                    <el-tag size="small" type="info" effect="plain">{{ e.source }}</el-tag>
                  </div>
                  <p class="text-xs font-mono text-[var(--text-secondary)] break-all">{{ e.sampleLine }}</p>
                </div>
              </div>
            </div>

            <!-- 结论 -->
            <div>
              <h4 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide mb-2">三、根因结论</h4>
              <p class="text-sm text-[var(--text-primary)] leading-relaxed mb-2">{{ reasonResult.conclusion?.rootCause }}</p>
              <div v-if="(reasonResult.conclusion?.suggestions || []).length > 0" class="mt-3">
                <p class="text-xs font-medium text-[var(--text-secondary)] mb-1">修复建议：</p>
                <ul class="list-disc list-inside space-y-1">
                  <li v-for="(s, i) in (reasonResult.conclusion?.suggestions || [])" :key="i" class="text-sm text-[var(--text-secondary)]">{{ s }}</li>
                </ul>
              </div>
              <div class="mt-3 flex items-center gap-2">
                <span class="text-xs text-muted">整体置信度:</span>
                <el-progress :percentage="Math.round((reasonResult.conclusion?.confidence || 0) * 100)" :stroke-width="6" style="width: 160px" />
              </div>
            </div>
          </div>
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

      <!-- 输入区 -->
      <div
        :class="[
          'chat-input-area px-4 pt-2',
          isMobile ? 'sticky bottom-16 pb-4' : 'pb-4'
        ]"
      >
        <!-- 日志项选择器 -->
        <div class="flex items-center gap-2 mb-2">
          <span class="text-xs text-muted flex-shrink-0">日志项:</span>
          <el-select
            v-model="selectedItemId"
            placeholder="选择日志项（可选）"
            size="small"
            clearable
            style="width: 240px"
            :loading="itemsLoading"
          >
            <el-option
              v-for="it in items"
              :key="it.id"
              :label="it.itemName"
              :value="it.id"
            />
          </el-select>
          <!-- 根因分析模式切换 -->
          <div class="flex items-center gap-1 ml-auto">
            <span class="text-xs text-muted">模式:</span>
            <el-radio-group v-model="reasoningMode" size="small">
              <el-radio-button label="normal">普通对话</el-radio-button>
              <el-radio-button label="reason">根因分析</el-radio-button>
            </el-radio-group>
          </div>
        </div>
        <div class="input-wrapper flex items-end gap-3 rounded-2xl p-3">
          <textarea
            ref="textareaRef"
            v-model="inputText"
            :rows="textareaRows"
            placeholder="输入检索问题，Enter 发送，Shift+Enter 换行"
            class="flex-1 resize-none bg-transparent border-none outline-none text-sm leading-relaxed py-1 placeholder:text-gray-400"
            data-testid="conversation-input"
            @input="autoResize"
            @keydown="handleKeyDown"
          />
          <button
            :class="[
              'flex-shrink-0 w-9 h-9 flex items-center justify-center rounded-full transition-colors',
              inputText.trim() && !loading
                ? 'bg-brand-600 text-white hover:bg-brand-700 shadow-sm'
                : 'bg-gray-200 text-gray-400 cursor-not-allowed'
            ]"
            :disabled="!inputText.trim() || loading"
            data-testid="conversation-send"
            @click="handleSend"
          >
            <svg v-if="!loading" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/>
            </svg>
            <svg v-else class="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
            </svg>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
/**
 * 对话式工作台 FR-1201 — v2.0+ 默认首页。增强版 UI。
 *
 * 特性: 欢迎界面、动画输入框、打字指示器、会话持久化、日志项选择器。
 * 移动端: 消息列表满宽、输入框 sticky bottom-16、安全区适配。
 */
import { ref, nextTick, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { searchLogs, createConversation, listConversations, getConversationMessages, appendConversationMessage, runAgentReasoning } from '@/api/logs'
import { nl2dslParse } from '@/api/nl2dsl'
import { queryAllItems } from '@/api/item'
import { useBreakpoint } from '@/composables/useBreakpoint'
import ChatMessage from '@/components/ChatMessage.vue'
import QueryResultCard from '@/components/QueryResultCard.vue'
import ExplanationCard from '@/components/ExplanationCard.vue'

const { isMobile } = useBreakpoint()
const route = useRoute()

const inputText = ref('')
const loading = ref(false)
const messages = ref([])
const chatContainerRef = ref(null)
const textareaRef = ref(null)
const textareaRows = ref(1)

// 会话持久化状态
const currentConvId = ref('')
const conversations = ref([])
const convLoading = ref(false)

// 日志项选择器状态
const selectedItemId = ref(null)
const items = ref([])
const itemsLoading = ref(false)

// 根因分析模式状态
const reasoningMode = ref('normal') // 'normal' | 'reason'
const reasonResult = ref(null)

let messageIdCounter = 0
function nextId() {
  return ++messageIdCounter
}

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

function formatConvTime(timeStr) {
  if (!timeStr) return ''
  // 后端返回格式 "2026-01-01 12:00:00" -> "01-01 12:00"
  const parts = timeStr.split(' ')
  if (parts.length >= 2) {
    const datePart = parts[0].slice(5) // MM-DD
    const timePart = parts[1].slice(0, 5) // HH:mm
    return `${datePart} ${timePart}`
  }
  return timeStr
}

function parseNaturalLanguage(text) {
  // 前端轻量级降级解析：当后端 NL2DSL 服务不可用时作为 fallback。
  // 主路径使用 nl2dslParse() 后端服务。
  const levelPattern = /\b(ERROR|WARN|INFO|DEBUG|FATAL)\b/i
  const levelMatch = text.match(levelPattern)
  const level = levelMatch ? levelMatch[1].toUpperCase() : ''
  const date = formatDate(new Date())
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
    const date2 = formatDate(threeDaysAgo)
    timeRangeDetail = `${date2} - ${formatDate(now)}`
  }
  let key = ''
  const quotedMatch = text.match(/["'"'"']([^"'"'"']+)["'"'"']/ )
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

function handleKeyDown(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

function autoResize() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  const lineHeight = 21 // ~1.5 line-height at 14px font
  const maxRows = 5
  const rows = Math.min(Math.max(1, Math.ceil(el.scrollHeight / lineHeight)), maxRows)
  textareaRows.value = rows
  el.style.height = rows === 1 ? 'auto' : `${rows * lineHeight}px`
}

async function scrollToBottom() {
  await nextTick()
  if (chatContainerRef.value) {
    chatContainerRef.value.scrollTop = chatContainerRef.value.scrollHeight
  }
}

function addUserMessage(text) {
  const msg = { id: nextId(), role: 'user', type: 'text', content: text, timestamp: formatTime(new Date()) }
  messages.value.push(msg)
  // 持久化到会话
  persistMessage('user', text)
  return msg
}

function addAssistantMessage(data) {
  const summary = data.summary || ''
  const msg = {
    id: nextId(), role: 'assistant', type: 'chart', content: summary,
    resultTotal: data.total || 0, rows: data.rows || [], truncated: data.truncated || false,
    timeRangeDetail: data.timeRangeDetail || '', explanation: data.explanation || null,
  }
  messages.value.push(msg)
  // 持久化到会话
  persistMessage('assistant', summary)
  return msg
}

function addErrorMessage(text, fallbackQuery) {
  const detail = `${text}${fallbackQuery ? ` (建议关键词: ${fallbackQuery})` : ''}`
  const msg = {
    id: nextId(), role: 'assistant', type: 'error', content: '',
    errorTitle: '查询遇到问题', errorDetail: detail, fallbackQuery: fallbackQuery || '',
  }
  messages.value.push(msg)
  // 持久化到会话
  persistMessage('assistant', `[错误] ${detail}`)
  return msg
}

// persistMessage 将消息追加到当前会话（如果有的话）。
async function persistMessage(role, content) {
  if (!currentConvId.value) return
  try {
    await appendConversationMessage(currentConvId.value, role, content)
  } catch {
    // 静默失败：会话持久化失败不影响用户体验
  }
}

async function executeSearch(queryText) {
  loading.value = true
  let params
  try {
    // Step 1: NL2DSL 解析（后端服务，替换硬编码正则）
    try {
      const nlRes = await nl2dslParse(queryText)
      const nlPayload = nlRes.data || {}
      if (nlPayload.code === 0 && nlPayload.data) {
        params = nlPayload.data
      } else {
        // NL2DSL 服务异常时降级为前端解析
        params = parseNaturalLanguage(queryText)
      }
    } catch {
      // 网络异常或 NL2DSL 不可用时降级
      params = parseNaturalLanguage(queryText)
    }

    const dslConditions = params.dslConditions || formatDslConditions(params)

    // Step 2: 执行日志检索（使用选择的日志项 ID）
    const res = await searchLogs({
      item: selectedItemId.value || 1,
      date: params.date,
      key: params.key,
      level: params.level,
      startTime: params.startTime,
      endTime: params.endTime,
      pageNo: 1,
      pageSize: 50,
    })
    const payload = res.data || {}
    if (payload.code && payload.code !== 0) {
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
    addErrorMessage('无法连接检索服务，请确认服务端已启动或网络可达。', params?.key || params?.level || '')
  } finally {
    loading.value = false
  }
}

function handleFallbackSearch(keyword) {
  if (keyword) { inputText.value = keyword; handleSend() }
}

// handleExampleClick 点击欢迎卡片示例标签：填入 + 自动发送。
function handleExampleClick(example) {
  if (example) {
    inputText.value = example
    handleSend()
  }
}

// executeReasoning 执行根因分析（调用 agent/reason 接口）。
async function executeReasoning(queryText) {
  loading.value = true
  reasonResult.value = null
  try {
    const res = await runAgentReasoning(queryText, currentConvId.value)
    const payload = res.data || {}
    if (payload.code && payload.code !== 0) {
      addErrorMessage(payload.msg || '根因分析失败，请稍后重试', '')
      return
    }
    reasonResult.value = payload.data || null
    // 持久化到会话
    if (reasonResult.value?.conclusion?.rootCause) {
      persistMessage('assistant', `[根因分析] ${reasonResult.value.conclusion.rootCause}`)
    }
  } catch {
    addErrorMessage('无法连接根因分析服务，请确认服务端已启动或网络可达。', '')
  } finally {
    loading.value = false
  }
}

async function handleSend() {
  const text = inputText.value.trim()
  if (!text || loading.value) return
  addUserMessage(text)
  inputText.value = ''
  textareaRows.value = 1
  await scrollToBottom()
  if (reasoningMode.value === 'reason') {
    await executeReasoning(text)
  } else {
    await executeSearch(text)
  }
  await scrollToBottom()
}

// ===== 会话管理 =====

// createNewSession 创建新会话并设置为当前会话。
async function createNewSession(title) {
  try {
    const { data } = await createConversation(title || '新会话')
    const convId = data?.data?.convId || ''
    if (convId) {
      currentConvId.value = convId
      // 刷新会话列表
      await loadConversations()
    }
    return convId
  } catch {
    // 创建失败不阻塞用户，仅静默处理
    return ''
  }
}

// loadConversations 加载会话列表。
async function loadConversations() {
  convLoading.value = true
  try {
    const { data } = await listConversations(1, 50)
    if (data && data.code === 0) {
      conversations.value = data.data?.list || []
    }
  } catch {
    // 加载失败不阻塞
  } finally {
    convLoading.value = false
  }
}

// switchConversation 切换到指定会话。
async function switchConversation(convId) {
  if (convId === currentConvId.value) return
  currentConvId.value = convId
  messages.value = []
  messageIdCounter = 0
  try {
    const { data } = await getConversationMessages(convId)
    if (data && data.code === 0) {
      const msgs = data.data || []
      for (const m of msgs) {
        const msg = {
          id: nextId(),
          role: m.role,
          type: m.role === 'user' ? 'text' : 'chart',
          content: m.content || '',
          timestamp: m.createdTime || '',
        }
        if (m.role === 'assistant' && m.content) {
          // 尝试从内容中解析结构化数据
          msg.resultTotal = m.tokenCount || 0
          msg.explanation = null
        }
        messages.value.push(msg)
      }
    }
  } catch {
    ElMessage.error('加载会话消息失败')
  }
  await scrollToBottom()
}

// handleNewConversation 新建会话按钮处理。
async function handleNewConversation() {
  currentConvId.value = ''
  messages.value = []
  messageIdCounter = 0
  inputText.value = ''
  await createNewSession()
  await loadConversations()
}

// loadItems 加载日志项列表。
async function loadItems() {
  itemsLoading.value = true
  try {
    const { data } = await queryAllItems()
    items.value = Array.isArray(data) ? data : data.data || []
  } catch {
    items.value = []
  } finally {
    itemsLoading.value = false
  }
}

// applyAlertContext 从 URL query 读取 alertId 参数，预填首轮问题。
function applyAlertContext() {
  const alertId = route.query.alertId
  const ruleName = route.query.ruleName
  if (alertId) {
    const ruleInfo = ruleName ? `（规则名: ${ruleName}）` : ''
    const presetQuestion = `告警规则 ID=${alertId}${ruleInfo} 触发了告警，请帮我诊断该告警的相关日志并分析可能的原因。`
    inputText.value = presetQuestion
    // 不自动发送，让用户确认后发送
  }
}

onMounted(async () => {
  // 加载日志项列表
  await loadItems()
  // 创建新会话
  await createNewSession()
  // 加载历史会话列表
  await loadConversations()
  // 处理 alertId 参数
  applyAlertContext()
})

defineExpose({ messages, inputText, parseNaturalLanguage, handleSend, reasoningMode, reasonResult })
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
.pb-safe-bottom {
  padding-bottom: env(safe-area-inset-bottom);
}

/* ===== 会话侧边栏 ===== */
.conversation-sidebar {
  width: 240px;
  border-right: 1px solid var(--border-light);
  display: flex;
  flex-direction: column;
  background: var(--bg-card-muted);
}
.sidebar-header {
  padding: 12px;
  border-bottom: 1px solid var(--border-light);
}
.sidebar-list {
  flex: 1;
  padding: 8px;
}
.conversation-item {
  padding: 10px 12px;
  margin-bottom: 4px;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--transition-fast);
}
.conversation-item:hover {
  background: var(--bg-card-hover);
}
.conversation-item.active {
  background: var(--brand-50);
  border: 1px solid var(--brand-200);
}
.conv-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-bottom: 4px;
}
.conv-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
  color: var(--text-muted);
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
</style>
