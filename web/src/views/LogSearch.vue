<template>
  <div>
    <h2 class="text-xl font-semibold mb-4">日志检索</h2>
    <el-card shadow="never" class="max-w-3xl">
      <el-form :model="form" label-width="100px">
        <el-form-item label="客户端">
          <el-select
            v-model="form.client"
            placeholder="选择客户端"
            style="width: 100%"
            @change="onClientChange"
          >
            <el-option label="全部客户端（并发检索）" :value="0" />
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
        <el-form-item label="日期">
          <el-date-picker
            v-model="form.date"
            type="date"
            value-format="YYYYMMDD"
            placeholder="检索哪一天的日志"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="关键词">
          <el-input v-model="form.key" placeholder="要检索的关键字或正则表达式" />
        </el-form-item>
        <el-form-item label="匹配模式">
          <el-checkbox v-model="form.regex">启用正则匹配</el-checkbox>
          <span class="text-gray-400 text-sm ml-2">勾选后关键词按正则解析</span>
        </el-form-item>
        <el-form-item label="上下文行数">
          <el-input-number v-model="form.line" :min="0" :max="500" />
          <span class="text-gray-400 text-sm ml-2">命中行上下的上下文行数</span>
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
        <el-form-item label="时间范围">
          <div class="flex gap-2">
            <el-time-picker
              v-model="form.startTime"
              format="HH:mm:ss"
              value-format="HH:mm:ss"
              placeholder="开始时间"
              :clearable="true"
              style="width: 100%"
            />
            <span class="text-gray-400 self-center">至</span>
            <el-time-picker
              v-model="form.endTime"
              format="HH:mm:ss"
              value-format="HH:mm:ss"
              placeholder="结束时间"
              :clearable="true"
              style="width: 100%"
            />
          </div>
        </el-form-item>
        <el-form-item label="高级检索">
          <el-switch
            v-model="form.advanced"
            active-text="使用布尔查询"
            inactive-text="普通关键词"
            @change="onAdvancedToggle"
          />
        </el-form-item>
        <el-form-item v-if="form.advanced" label="布尔查询">
          <el-input
            v-model="form.query"
            type="textarea"
            :rows="2"
            placeholder="E.g. ERROR AND timeout NOT level:DEBUG&#10;支持 field:value (level/ip/traceId/module) AND/OR/NOT ()"
          />
          <div class="text-gray-400 text-xs mt-1">
            支持的字段：<code>level</code> <code>ip</code> <code>traceId</code> <code>module</code>；操作符：<code>AND</code> <code>OR</code> <code>NOT</code> <code>( )</code>
          </div>
        </el-form-item>
        <el-form-item label="历史记录">
          <el-select
            v-model="selectedHistory"
            placeholder="选择历史查询"
            clearable
            style="width: 100%"
            @change="applyHistory"
          >
            <el-option
              v-for="(h, idx) in searchHistory"
              :key="idx"
              :label="formatHistoryLabel(h)"
              :value="idx"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :icon="Search" :loading="searching" @click="onSearch">
            开始检索
          </el-button>
          <el-button :icon="Refresh" @click="resetForm" class="ml-2">
            清空
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 在线分页结果卡片 -->
    <el-card v-if="rows.length > 0 || searching || searched" shadow="never" class="mt-6">
      <div class="flex items-center justify-between mb-3">
        <div class="text-sm text-gray-600">
          共命中
          <span class="font-semibold text-gray-800">{{ resultTotal }}</span>
          行（当前展示 {{ rows.length }} 行）
        </div>
        <el-alert
          v-if="resultTruncated"
          type="warning"
          :closable="false"
          show-icon
          title="命中行过多，仅展示前 100000 行。请缩小时间范围或提高关键词精度以获取完整结果。"
          class="mb-3"
        />
        <div class="flex items-center gap-3">
          <span v-if="searching" class="text-primary-500 text-sm">加载中…</span>
          <el-button
            v-if="currentTaskNo"
            :icon="Link"
            size="small"
            @click="router.push('/tasks')"
          >
            任务 {{ currentTaskNo }}
          </el-button>
          <el-button
            v-if="lastSearchMeta"
            :icon="Download"
            :loading="downloading"
            size="small"
            @click="onDownloadZip"
          >
            下载 zip
          </el-button>
          <el-button
            v-if="lastSearchMeta"
            :icon="VideoPlay"
            size="small"
            @click="gotoTail"
          >
            定位到 Tail
          </el-button>
          <el-button
            v-if="lastSearchMeta && form.key"
            :icon="Share"
            size="small"
            @click="gotoTrace"
          >
            追踪链路
          </el-button>
        </div>
      </div>
      <el-table :data="rows" stripe size="small" max-height="480" style="width: 100%">
        <el-table-column type="index" label="#" width="55" />
        <el-table-column prop="source" label="来源" width="160" show-overflow-tooltip />
        <el-table-column prop="line" label="日志行" min-width="400">
          <template #default="{ row }">
            <code class="whitespace-pre-wrap break-all text-xs">{{ row.line }}</code>
          </template>
        </el-table-column>
      </el-table>
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

/** 日志检索页：选择客户端 + 日志项，按日期和关键词跨节点检索日志。
 *  支持正则匹配、日志级别过滤、时间范围过滤和历史记录功能。
 *  结果以 zip 下载，错误以 JSON 返回还原。
 */
<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Search, Refresh, Download, Link, VideoPlay, Share } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { queryLogClients, queryItemsByClient, queryLogs, searchLogs } from '@/api/logs'
import { queryAllItems } from '@/api/item'

const HISTORY_KEY = 'ydsz-log-search-history'
const MAX_HISTORY = 10

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const searching = ref(false)
const searchHistory = ref([])
const selectedHistory = ref(null)

// 在线分页状态
const rows = ref([])
const pageNo = ref(1)
const pageSize = ref(50)
const resultTotal = ref(0)
const resultTruncated = ref(false)
const downloading = ref(false)
const searched = ref(false)
const lastSearchMeta = ref(null)
const currentTaskNo = ref('')
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

// 历史记录操作
function loadHistory() {
  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    searchHistory.value = raw ? JSON.parse(raw) : []
  } catch {
    searchHistory.value = []
  }
}

function saveHistory() {
  // 完全相同的查询不重复保存
  const dupIdx = searchHistory.value.findIndex(h =>
    h.item === form.item && h.date === form.date && h.key === form.key &&
    h.regex === form.regex && h.level === form.level &&
    h.startTime === form.startTime && h.endTime === form.endTime &&
    !!h.advanced === !!form.advanced && h.query === form.query
  )
  if (dupIdx >= 0) {
    searchHistory.value.splice(dupIdx, 1)
  }
  // 新查询插入到最前
  searchHistory.value.unshift({
    item: form.item,
    itemName: items.value.find(it => it.id === form.item)?.itemName || '未知',
    client: form.client,
    clientName: form.client === 0 ? '全部客户端' :
      (clients.value.find(c => c.id === form.client)?.ip + ':' +
       clients.value.find(c => c.id === form.client)?.port) || '',
    date: form.date,
    key: form.key,
    line: form.line,
    regex: form.regex,
    level: form.level,
    startTime: form.startTime,
    endTime: form.endTime,
    advanced: form.advanced,
    query: form.query || '',
    ts: Date.now(),
  })
  // 仅保留最近 N 条
  if (searchHistory.value.length > MAX_HISTORY) {
    searchHistory.value = searchHistory.value.slice(0, MAX_HISTORY)
  }
  localStorage.setItem(HISTORY_KEY, JSON.stringify(searchHistory.value))
  selectedHistory.value = null
}

function applyHistory(idx) {
  if (idx === null || idx < 0 || idx >= searchHistory.value.length) return
  const h = searchHistory.value[idx]
  form.item = h.item
  form.client = h.client
  form.date = h.date
  form.key = h.key || ''
  form.line = h.line ?? 20
  form.regex = !!h.regex
  form.level = h.level || ''
  form.startTime = h.startTime || ''
  form.endTime = h.endTime || ''
  form.advanced = !!h.advanced
  form.query = h.query || ''
  // 切换模式时保持两侧一致性
  if (form.advanced) form.key = ''
  else form.query = ''
}

function formatHistoryLabel(h) {
  const parts = []
  parts.push(h.date || '-')
  parts.push(h.advanced && h.query ? `[布尔] ${h.query.length > 20 ? h.query.slice(0, 20) + '…' : h.query}` : (h.key || '-'))
  if (h.level) parts.push(`[${h.level}]`)
  if (h.regex) parts.push('(正则)')
  const timeRange = h.startTime || h.endTime ? `${h.startTime || '00:00'}-${h.endTime || '23:59'}` : ''
  if (timeRange) parts.push(`@${timeRange}`)
  const ts = h.ts ? new Date(h.ts).toLocaleString() : ''
  return `${parts.join(' ')}  — ${ts}`
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
  selectedHistory.value = null
}

function onAdvancedToggle() {
  // 切换模式时清空另一边，避免二者并存导致非预期交集。
  if (form.advanced) {
    form.key = ''
  } else {
    form.query = ''
  }
}

async function onSearch() {
  if (!form.item) {
    ElMessage.warning('请选择日志项')
    return
  }
  if (!form.date) {
    ElMessage.warning('请选择日期')
    return
  }
  if (form.advanced && !form.query.trim()) {
    ElMessage.warning('请输入布尔查询表达式')
    return
  }
  if (!form.advanced && !form.key.trim()) {
    ElMessage.warning('请输入检索关键词')
    return
  }
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
    currentTaskNo.value = data.taskNo || ''
    searched.value = true
    lastSearchMeta.value = {
      client: form.client || 0,
      item: form.item,
      date: form.date,
      key: form.key,
      line: form.line,
      regex: form.regex,
      level: form.level,
      startTime: form.startTime,
      endTime: form.endTime,
      advanced: form.advanced,
      query: form.query || '',
    }
    // 保存查询历史
    saveHistory()
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

async function fetchPage() {
  if (!lastSearchMeta.value) return
  const meta = lastSearchMeta.value
  searching.value = true
  try {
    const res = await searchLogs({
      client: meta.client,
      item: meta.item,
      date: meta.date,
      key: meta.key,
      line: meta.line,
      regex: meta.regex,
      level: meta.level,
      startTime: meta.startTime,
      endTime: meta.endTime,
      query: meta.query || '',
      pageNo: pageNo.value,
      pageSize: pageSize.value,
    })
    const payload = res.data || {}
    const data = payload.data || {}
    rows.value = data.list || []
    resultTotal.value = data.total || 0
    resultTruncated.value = !!data.truncated
    currentTaskNo.value = data.taskNo || currentTaskNo.value
  } catch {
    ElMessage.error('分页查询失败，请重试')
  } finally {
    searching.value = false
  }
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
      client: meta.client,
      item: meta.item,
      date: meta.date,
      key: meta.key,
      line: meta.line,
      regex: meta.regex,
      level: meta.level,
      startTime: meta.startTime,
      endTime: meta.endTime,
      query: meta.query || '',
    })
    const blob = res.data
    const ct = res.headers['content-type'] || ''
    if (ct.includes('application/json')) {
      const text = await blob.text()
      try {
        const obj = JSON.parse(text)
        ElMessage.error(obj.msg || '检索失败')
      } catch {
        ElMessage.error('检索失败')
      }
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
  loadHistory()
})
</script>
