<template>
  <div class="logs-page">
    <!-- 统一页面头部 -->
    <div class="page-header mb-5">
      <h2 class="page-header__title">
        <span class="page-header__icon" style="background: linear-gradient(135deg, #10b981, #059669); box-shadow: 0 2px 8px rgba(16, 185, 129, 0.2);">
          <el-icon :size="18"><Search /></el-icon>
        </span>
        历史日志检索
      </h2>
      <p class="page-header__subtitle">选择客户端与日志项，跨节点检索日志</p>
    </div>

    <!-- 检索表单卡片 -->
    <el-card shadow="never" class="!rounded-xl search-card mb-5">
      <div class="search-form">
        <el-form :model="form" label-position="top" class="grid-form">
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
          <el-form-item label="日志项" required>
            <el-select
              v-model="form.item"
              placeholder="请选择日志项"
              style="width: 100%"
              :loading="itemsLoading"
            >
              <el-option v-for="it in items" :key="it.id" :label="it.itemName" :value="it.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="日期" required>
            <el-date-picker
              v-model="form.date"
              type="date"
              value-format="YYYYMMDD"
              placeholder="选择检索日期"
              style="width: 100%"
            />
          </el-form-item>
        </el-form>

        <el-form :model="form" label-position="top">
          <el-form-item label="关键词">
            <div class="relative w-full">
              <el-input
                v-model="form.key"
                placeholder="要检索的关键字或正则表达式"
                size="default"
                @focus="handleKeywordFocus"
                @blur="handleKeywordBlur"
              />
              <button
                class="history-btn absolute right-2 top-1/2 -translate-y-1/2 flex items-center text-muted hover:text-brand-600 transition-colors"
                title="搜索历史"
                @mousedown.prevent="handleKeywordFocus"
              >
                <el-icon><Clock /></el-icon>
              </button>
              <SearchHistoryDropdown
                ref="historyDropdownRef"
                @select="handleHistorySelect"
              />
            </div>
          </el-form-item>
        </el-form>

        <div class="flex flex-wrap items-center gap-3">
          <el-checkbox v-model="form.regex" class="!mr-0">
            <span class="text-sm">正则匹配</span>
          </el-checkbox>
          <el-select v-model="form.level" placeholder="不过滤级别" clearable style="width: 140px">
            <el-option label="DEBUG" value="DEBUG" />
            <el-option label="INFO" value="INFO" />
            <el-option label="WARN" value="WARN" />
            <el-option label="ERROR" value="ERROR" />
            <el-option label="FATAL" value="FATAL" />
          </el-select>
          <div class="time-range flex items-center gap-2">
            <el-time-picker
              v-model="form.startTime"
              format="HH:mm:ss"
              value-format="HH:mm:ss"
              placeholder="开始"
              :clearable="true"
              style="width: 140px"
            />
            <span class="text-muted text-xs">至</span>
            <el-time-picker
              v-model="form.endTime"
              format="HH:mm:ss"
              value-format="HH:mm:ss"
              placeholder="结束"
              :clearable="true"
              style="width: 140px"
            />
          </div>
        </div>

        <div class="flex items-center gap-3">
          <el-button type="primary" :icon="Search" :loading="searching" @click="onSearch">
            开始检索
          </el-button>
          <el-button :icon="Refresh" @click="resetForm">清空</el-button>
          <el-button text :icon="Clock" @click="toggleHistoryDropdown">搜索历史</el-button>
        </div>
      </div>
    </el-card>

    <!-- 在线分页结果卡片 -->
    <el-card v-if="rows.length > 0 || searching || searched" shadow="never" class="!rounded-xl">
      <!-- 结果头部 -->
      <div class="result-header flex items-center justify-between mb-4">
        <div class="flex items-center gap-3">
          <div class="result-stat flex items-center gap-1.5">
            <el-icon class="text-success"><CircleCheckFilled /></el-icon>
            <span class="text-sm text-secondary">
              共命中 <strong class="text-primary">{{ resultTotal }}</strong> 行
            </span>
          </div>
          <span class="text-xs text-muted">当前展示 {{ rows.length }} 行</span>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="searching" class="flex items-center gap-1 text-brand-500 text-sm">
            <el-icon class="animate-spin"><Loading /></el-icon>搜索中…
          </span>
          <el-button v-if="currentTaskNo" :icon="Link" size="small" @click="router.push('/tasks')">任务详情</el-button>
          <el-button v-if="lastSearchMeta" :icon="Download" :loading="downloading" size="small" @click="onDownloadZip">下载 zip</el-button>
          <el-button v-if="lastSearchMeta" :icon="VideoPlay" size="small" @click="gotoTail">定位 Tail</el-button>
          <el-button v-if="lastSearchMeta && form.key" :icon="Share" size="small" @click="gotoTrace">追踪链路</el-button>
        </div>
      </div>

      <el-alert
        v-if="resultTruncated"
        type="warning"
        :closable="false"
        show-icon
        class="mb-4"
      >
        <template #title>
          结果已达上限 {{ resultTotal }} 行，建议缩小时间范围或添加更多过滤条件。
        </template>
      </el-alert>

      <!-- 模板聚类视图 -->
      <div v-if="templateGroups.length > 0" class="mb-4">
        <div class="flex items-center justify-between mb-2">
          <span class="text-sm text-secondary font-medium">模板聚类视图</span>
          <el-radio-group v-model="viewMode" size="small">
            <el-radio-button value="cluster">聚类</el-radio-button>
            <el-radio-button value="flat">原始列表</el-radio-button>
          </el-radio-group>
        </div>

        <el-collapse v-if="viewMode === 'cluster'" v-model="activeTemplateGroups">
          <el-collapse-item
            v-for="g in templateGroups"
            :key="g.templateId"
            :name="g.templateId"
          >
            <template #title>
              <div class="flex items-center gap-2 w-full">
                <code class="text-xs text-secondary font-mono truncate max-w-md">{{ g.template }}</code>
                <el-tag size="small" type="info">{{ g.count }} 次</el-tag>
                <el-tag v-if="g.level" size="small" :type="getLevelTagType(g.level)">{{ g.level }}</el-tag>
              </div>
            </template>
            <div class="bg-muted-bg p-3 rounded-lg text-sm font-mono">
              <div
                v-for="(line, i) in (expandedTemplates[g.templateId] ? g.sampleLines : g.sampleLines.slice(0, 5))"
                :key="i"
                class="text-secondary leading-6 py-0.5"
              >{{ line }}</div>
              <el-button
                v-if="g.sampleLines.length > 5 && !expandedTemplates[g.templateId]"
                text
                size="small"
                class="mt-1"
                @click="expandedTemplates[g.templateId] = true"
              >
                展开全部 ({{ g.sampleLines.length }} 条)
              </el-button>
            </div>
          </el-collapse-item>
        </el-collapse>
      </div>

      <!-- 日志直方图 -->
      <LogHistogram
        v-if="rows.length > 0"
        :rows="rows"
        class="mb-4"
        @update="handleHistogramUpdate"
      />

      <!-- 日志表格 -->
      <el-table
        v-if="viewMode === 'flat' || templateGroups.length === 0"
        :data="rows"
        stripe
        size="small"
        max-height="480"
        style="width: 100%"
      >
        <el-table-column type="index" label="#" width="55" />
        <el-table-column prop="source" label="来源" width="160" show-overflow-tooltip />
        <el-table-column prop="line" label="日志行" min-width="400">
          <template #default="{ row, $index }">
            <div class="log-cell">
              <code class="whitespace-pre-wrap break-all text-xs log-cell-content">{{ row.line }}</code>
              <el-button
                :icon="DocumentCopy"
                size="small"
                class="log-copy-btn"
                @click.stop="handleCopy($index)"
              />
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- 加载更多 -->
      <div v-if="hasMore && !searching" class="mt-4 flex justify-center">
        <el-button :loading="loadingMore" @click="handleLoadMore">
          加载更多（已加载 {{ rows.length }} / {{ resultTotal }}）
        </el-button>
      </div>
      <div v-if="resultTotal > pageSize" class="mt-4 flex justify-end">
        <el-pagination
          v-model:current-page="pageNo"
          v-model:page-size="pageSize"
          :page-sizes="[50, 100, 200]"
          :total="resultTotal"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @current-change="onPageChange"
          @size-change="onSizeChange"
        />
      </div>
      <el-empty v-if="!searching && rows.length === 0 && searched" description="未找到匹配行" />
    </el-card>
  </div>
</template>

<script setup>
/** 历史日志检索页 v2.0：选择客户端 + 日志项，按日期和关键词跨节点检索日志。 */
import { ref, reactive, shallowRef, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Search, Refresh, Download, Link, VideoPlay, Share, DocumentCopy, Clock, CircleCheckFilled, Loading } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { queryLogClients, queryItemsByClient, queryLogs, searchLogs } from '@/api/logs'
import { queryAllItems } from '@/api/item'
import LogHistogram from '@/components/LogHistogram.vue'
import { useSearchHistory } from '@/store/searchHistory'
import SearchHistoryDropdown from '@/components/SearchHistoryDropdown.vue'

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const searching = ref(false)

const viewMode = ref('cluster')
const templateGroups = ref([])
const activeTemplateGroups = ref([])
const expandedTemplates = ref({})

const searchStore = useSearchHistory()
const historyDropdownRef = ref(null)
const showHistoryDropdown = ref(false)

const rows = shallowRef([])
const pageNo = ref(1)
const pageSize = ref(50)
const resultTotal = ref(0)
const resultTruncated = ref(false)
const downloading = ref(false)
const searched = ref(false)
const lastSearchMeta = ref(null)
const currentTaskNo = ref('')
const hasMore = ref(false)
const loadingMore = ref(false)
const router = useRouter()

const form = reactive({
  client: 0,
  item: null,
  date: '',
  key: '',
  line: 20,
  regex: false,
  level: '',
  startTime: '',
  endTime: '',
  advanced: false,
  query: '',
})

async function loadClients() {
  const { data } = await queryLogClients()
  clients.value = data.data || []
}

async function loadItems() {
  itemsLoading.value = true
  try {
    if (form.client && form.client !== 0) {
      const { data } = await queryItemsByClient(form.client)
      items.value = data.data || []
    } else {
      const { data } = await queryAllItems()
      items.value = Array.isArray(data) ? data : data.data || []
    }
  } finally {
    itemsLoading.value = false
  }
}

function onClientChange() {
  form.item = null
  loadItems()
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

function handleHistorySelect(entry) {
  form.key = entry.query
  if (entry.params) {
    if (entry.params.item) form.item = entry.params.item
    if (entry.params.client !== undefined) form.client = entry.params.client
    if (entry.params.date) form.date = entry.params.date
    if (entry.params.line !== undefined) form.line = entry.params.line
    if (entry.params.regex !== undefined) form.regex = entry.params.regex
    if (entry.params.level !== undefined) form.level = entry.params.level
    if (entry.params.startTime) form.startTime = entry.params.startTime
    if (entry.params.endTime) form.endTime = entry.params.endTime
  }
}

function handleKeywordFocus() {
  showHistoryDropdown.value = true
  if (historyDropdownRef.value) historyDropdownRef.value.show()
}

function handleKeywordBlur() {
  setTimeout(() => {
    showHistoryDropdown.value = false
    if (historyDropdownRef.value) historyDropdownRef.value.hide()
  }, 150)
}

function toggleHistoryDropdown() {
  if (!historyDropdownRef.value) return
  if (showHistoryDropdown.value) {
    historyDropdownRef.value.hide()
  } else {
    showHistoryDropdown.value = true
    historyDropdownRef.value.show()
  }
}

function addHistoryToStore() {
  const query = form.advanced ? form.query : form.key
  if (!query || !query.trim()) return
  searchStore.add({
    query: query.trim(),
    timestamp: Date.now(),
    hitCount: resultTotal.value || 0,
    params: {
      item: form.item,
      itemName: items.value.find(it => it.id === form.item)?.itemName || '',
      client: form.client,
      clientName: (() => {
        if (form.client === 0) return '全部客户端'
        const c = clients.value.find(c => c.id === form.client)
        return c ? `${c.ip}:${c.port}` : ''
      })(),
      date: form.date,
      line: form.line,
      regex: form.regex,
      level: form.level,
      startTime: form.startTime,
      endTime: form.endTime,
      advanced: form.advanced,
    },
  })
}

function getLevelTagType(level) {
  switch (level) {
    case 'ERROR':
    case 'FATAL': return 'danger'
    case 'WARN': return 'warning'
    case 'INFO': return 'success'
    case 'DEBUG': return 'info'
    default: return ''
  }
}

function resetForm() {
  form.client = 0
  form.item = null
  form.date = ''
  form.key = ''
  form.line = 20
  form.regex = false
  form.level = ''
  form.startTime = ''
  form.endTime = ''
  form.advanced = false
  form.query = ''
  templateGroups.value = []
  activeTemplateGroups.value = []
  expandedTemplates.value = {}
}

async function onSearch() {
  if (!form.item) { ElMessage.warning('请选择日志项'); return }
  if (!form.date) { ElMessage.warning('请选择日期'); return }
  if (form.advanced && !form.query.trim()) { ElMessage.warning('请输入布尔查询表达式'); return }
  if (!form.advanced && !form.key.trim()) { ElMessage.warning('请输入检索关键词'); return }
  pageNo.value = 1
  searched.value = false
  rows.value = []
  resultTotal.value = 0
  resultTruncated.value = false
  lastSearchMeta.value = null
  searching.value = true
  try {
    const res = await searchLogs({
      client: form.client || 0,
      item: form.item,
      date: form.date,
      key: form.key,
      line: form.line,
      regex: form.regex,
      level: form.level,
      startTime: form.startTime,
      endTime: form.endTime,
      query: form.query || '',
      pageNo: pageNo.value,
      pageSize: pageSize.value,
    })
    const payload = res.data || {}
    if (payload.code && payload.code !== '200') {
      ElMessage.error(payload.msg || '检索失败')
      return
    }
    const data = payload.data || {}
    rows.value = data.list || []
    resultTotal.value = data.total || 0
    resultTruncated.value = !!data.truncated
    hasMore.value = data.total > pageSize.value * pageNo.value
    currentTaskNo.value = data.taskNo || ''
    templateGroups.value = data.templateGroups || []
    activeTemplateGroups.value = templateGroups.value.slice(0, 3).map(g => g.templateId)
    expandedTemplates.value = {}
    searched.value = true
    lastSearchMeta.value = {
      client: form.client || 0, item: form.item, date: form.date, key: form.key,
      line: form.line, regex: form.regex, level: form.level,
      startTime: form.startTime, endTime: form.endTime,
      advanced: form.advanced, query: form.query || '',
    }
    addHistoryToStore()
    if (resultTotal.value === 0) {
      ElMessage.info('检索完成，未匹配到结果')
    } else {
      ElMessage.success(`检索完成，共 ${resultTotal.value} 行`)
    }
  } catch {
    ElMessage.error('检索请求失败，请确认服务端已启动')
  } finally {
    searching.value = false
  }
}

async function onPageChange(page) {
  pageNo.value = page
  await fetchPage()
}

async function onSizeChange(size) {
  pageSize.value = size
  pageNo.value = 1
  await fetchPage()
}

async function fetchPage(append = false) {
  if (!lastSearchMeta.value) return
  const meta = lastSearchMeta.value
  searching.value = true
  try {
    const res = await searchLogs({
      client: meta.client, item: meta.item, date: meta.date, key: meta.key,
      line: meta.line, regex: meta.regex, level: meta.level,
      startTime: meta.startTime, endTime: meta.endTime,
      query: meta.query || '',
      pageNo: pageNo.value, pageSize: pageSize.value,
    })
    const payload = res.data || {}
    const data = payload.data || {}
    if (append) {
      rows.value = rows.value.concat(data.list || [])
    } else {
      rows.value = data.list || []
    }
    resultTotal.value = data.total || 0
    resultTruncated.value = !!data.truncated
    hasMore.value = data.total > pageSize.value * pageNo.value
    currentTaskNo.value = data.taskNo || currentTaskNo.value
    if (data.templateGroups) {
      templateGroups.value = data.templateGroups
      activeTemplateGroups.value = data.templateGroups.slice(0, 3).map(g => g.templateId)
      expandedTemplates.value = {}
    }
  } catch {
    ElMessage.error('分页查询失败，请重试')
  } finally {
    searching.value = false
  }
}

async function handleHistogramUpdate(range) {
  form.startTime = range.startTime
  form.endTime = range.endTime
  pageNo.value = 1
  rows.value = []
  searched.value = false
  await onSearch()
}

async function handleCopy(index) {
  const row = rows.value[index]
  const text = typeof row === 'string' ? row : (row.line || row.content || JSON.stringify(row))
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

async function handleLoadMore() {
  if (!lastSearchMeta.value) return
  loadingMore.value = true
  pageNo.value++
  try { await fetchPage(true) } finally { loadingMore.value = false }
}

function gotoTail() {
  if (!lastSearchMeta.value) return
  const meta = lastSearchMeta.value
  const q = new URLSearchParams()
  if (meta.client && meta.client !== 0) q.set('client', String(meta.client))
  if (meta.item) q.set('item', String(meta.item))
  if (meta.date) q.set('date', meta.date)
  if (meta.key) q.set('key', meta.key)
  if (meta.level) q.set('level', meta.level)
  if (meta.regex) q.set('regex', '1')
  router.push('/logs/tail?' + q.toString())
}

function gotoTrace() {
  if (!form.key) return
  const q = new URLSearchParams()
  q.set('traceId', form.key.trim())
  if (form.date) q.set('date', form.date)
  if (form.item) q.set('item', String(form.item))
  if (form.client && form.client !== 0) q.set('client', String(form.client))
  if (form.regex) q.set('regex', '1')
  router.push('/trace?' + q.toString())
}

async function onDownloadZip() {
  if (!lastSearchMeta.value) return
  const meta = lastSearchMeta.value
  downloading.value = true
  try {
    const res = await queryLogs({
      client: meta.client, item: meta.item, date: meta.date,
      key: meta.key, line: meta.line, regex: meta.regex,
      level: meta.level, startTime: meta.startTime,
      endTime: meta.endTime, query: meta.query || '',
    })
    const blob = res.data
    const ct = res.headers['content-type'] || ''
    if (ct.includes('application/json')) {
      const text = await blob.text()
      try {
        const obj = JSON.parse(text)
        ElMessage.error(obj.msg || '检索失败')
      } catch { ElMessage.error('检索失败') }
      return
    }
    downloadBlob(blob, `${meta.date || 'log'}_${meta.key || 'all'}.zip`)
    ElMessage.success('已开始下载结果压缩包')
  } catch {
    ElMessage.error('下载失败，请确认服务端已启动')
  } finally {
    downloading.value = false
  }
}

onMounted(() => {
  loadClients()
  loadItems()
  searchStore.load()
})
</script>

<style scoped>
/* ===== 表单卡片 ===== */
.search-card :deep(.el-form-item__label) {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-secondary);
  padding-bottom: 4px;
}
.grid-form {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 0 20px;
}
@media (max-width: 768px) {
  .grid-form {
    grid-template-columns: 1fr;
  }
}

/* 检索历史按钮 */
.history-btn {
  background: none;
  border: none;
  cursor: pointer;
  padding: 4px;
  z-index: 2;
}

/* ===== 结果头部 ===== */
.result-header {
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-light);
  margin-bottom: 16px;
}
.result-stat .text-primary {
  color: var(--brand-600);
  font-size: 16px;
}

/* 日志单元格 */
.log-cell {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 4px;
}
.log-copy-btn {
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.2s;
  padding: 4px;
}
.log-cell:hover .log-copy-btn { opacity: 1; }
.log-cell-content { flex: 1; }

/* 背景色 */
.bg-muted-bg {
  background: var(--bg-card-muted);
}
</style>
