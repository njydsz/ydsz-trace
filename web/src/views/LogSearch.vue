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
  </div>
</template>

/** 日志检索页：选择客户端 + 日志项，按日期和关键词跨节点检索日志。
 *  支持正则匹配、日志级别过滤、时间范围过滤和历史记录功能。
 *  结果以 zip 下载，错误以 JSON 返回还原。
 */
<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Search, Refresh } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { queryLogClients, queryItemsByClient, queryLogs } from '@/api/logs'
import { queryAllItems } from '@/api/item'

const HISTORY_KEY = 'ydsz-log-search-history'
const MAX_HISTORY = 10

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const searching = ref(false)
const searchHistory = ref([])
const selectedHistory = ref(null)

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
    h.startTime === form.startTime && h.endTime === form.endTime
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
  if (idx == null || idx < 0 || idx >= searchHistory.value.length) return
  const h = searchHistory.value[idx]
  form.item = h.item
  form.client = h.client
  form.date = h.date
  form.key = h.key
  form.line = h.line ?? 20
  form.regex = !!h.regex
  form.level = h.level || ''
  form.startTime = h.startTime || ''
  form.endTime = h.endTime || ''
}

function formatHistoryLabel(h) {
  const parts = []
  parts.push(h.date || '-')
  parts.push(h.key || '-')
  if (h.level) parts.push(`[${h.level}]`)
  if (h.regex) parts.push('(正则)')
  const timeRange = h.startTime || h.endTime
    ? `${h.startTime || '00:00'}-${h.endTime || '23:59'}`
    : ''
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
  selectedHistory.value = null
}

async function onSearch() {
  if (!form.item) {
    ElMessage.warning('请选择日志项')
    return
  }
  if (!form.key) {
    ElMessage.warning('请输入检索关键词')
    return
  }
  if (!form.date) {
    ElMessage.warning('请选择日期')
    return
  }
  searching.value = true
  try {
    const res = await queryLogs({
      client: form.client || 0,
      item: form.item,
      date: form.date,
      key: form.key,
      line: form.line,
      regex: form.regex,
      level: form.level,
      startTime: form.startTime,
      endTime: form.endTime,
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
    downloadBlob(blob, `${form.key}.zip`)
    ElMessage.success('检索完成，已开始下载结果压缩包')
    // 保存查询历史
    saveHistory()
  } catch {
    ElMessage.error('检索请求失败，请确认服务端已启动')
  } finally {
    searching.value = false
  }
}

onMounted(() => {
  loadClients()
  loadItems()
  loadHistory()
})
</script>
