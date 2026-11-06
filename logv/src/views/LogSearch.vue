<template>
  <div class="logs-page">
    <!-- 统一页面头部 -->
    <div class="page-header mb-5">
      <h2 class="page-header__title" data-testid="log-search-title">
        <span class="page-header__icon" style="background: linear-gradient(135deg, #10b981, #059669); box-shadow: 0 2px 8px rgba(16, 185, 129, 0.2);">
          <el-icon :size="18"><Search /></el-icon>
        </span>
        历史日志检索
      </h2>
      <p class="page-header__subtitle" data-testid="log-search-subtitle">选择客户端与日志项，跨节点检索日志</p>
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
                data-testid="log-search-keyword"
                @focus="handleKeywordFocus"
                @blur="handleKeywordBlur"
                @input="onTemplateInput"
              />
              <!-- 模板自动补全下拉 -->
              <div
                v-if="templateSuggestionsVisible && templateSuggestions.length > 0"
                class="template-autocomplete absolute left-0 right-0 top-full z-20 mt-1 max-h-60 overflow-y-auto rounded-lg border border-light bg-card shadow-lg"
                data-testid="log-search-template-suggestions"
              >
                <div
                  v-for="(tpl, i) in templateSuggestions"
                  :key="tpl.templateId || i"
                  class="template-autocomplete__item cursor-pointer border-b border-light px-3 py-2 transition-colors last:border-b-0"
                  data-testid="log-search-template-item"
                  @mousedown.prevent="selectTemplateSuggestion(tpl)"
                >
                  <code class="block text-xs font-mono text-secondary leading-5 break-all">{{ tpl.templateText || tpl.sampleLine }}</code>
                  <div class="mt-1 flex items-center gap-2">
                    <el-tag v-if="tpl.level" size="small" :type="getLevelTagType(tpl.level)">{{ tpl.level }}</el-tag>
                    <span class="text-xs text-muted">命中 {{ tpl.totalCount || 0 }} 次</span>
                  </div>
                </div>
              </div>
              <button
                class="history-btn absolute right-2 top-1/2 -translate-y-1/2 flex items-center text-muted hover:text-brand-600 transition-colors"
                title="搜索历史"
                data-testid="log-search-history-btn"
                @mousedown.prevent="handleKeywordFocus"
              >
                <el-icon><Clock /></el-icon>
              </button>
              <SearchHistoryDropdown
                ref="historyDropdownRef"
                data-testid="log-search-history-dropdown"
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
          <el-button type="primary" :icon="Search" :loading="searching" data-testid="log-search-submit" @click="onSearch">
            开始检索
          </el-button>
          <el-button :icon="Refresh" data-testid="log-search-reset" @click="resetForm">清空</el-button>
          <el-button text :icon="Clock" data-testid="log-search-toggle-history" @click="toggleHistoryDropdown">搜索历史</el-button>
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

          <!-- 实时进度条（多节点检索时显示） -->
          <div v-if="searching && taskNodeTotal > 1" class="flex items-center gap-2 ml-3">
            <el-progress
              :percentage="taskProgressPct"
              :stroke-width="6"
              style="width: 120px"
              :format="() => `${taskNodeDone}/${taskNodeTotal}`"
            />
            <span v-if="taskEstimatedTotal > 0" class="text-xs text-muted">
              预估 {{ taskEstimatedTotal.toLocaleString() }} 行
            </span>
          </div>

          <!-- 圆环进度（单节点检索时显示，避免与上方进度条重复） -->
          <TaskProgressRing
            v-if="searching && taskNodeTotal <= 1"
            :percent="taskProgressPct"
            :size="26"
            class="ml-2"
          />
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
          <div class="flex items-center gap-2">
            <!-- V2.4-09：排序切换（后端默认按命中次数降序） -->
            <el-radio-group v-model="groupSort" size="small">
              <el-radio-button value="count">按次数</el-radio-button>
              <el-radio-button value="time">按时间</el-radio-button>
            </el-radio-group>
            <el-radio-group v-model="viewMode" size="small">
              <el-radio-button value="cluster">聚类</el-radio-button>
              <el-radio-button value="flat">原始列表</el-radio-button>
            </el-radio-group>
          </div>
        </div>

        <el-collapse v-if="viewMode === 'cluster'" v-model="activeTemplateGroups">
          <el-collapse-item
            v-for="g in sortedTemplateGroups"
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
              <!-- V2.4-09：后端样本上限为 3 条，展开阈值需与其一致，
                   否则永远显示不出「展开全部」按钮 -->
              <div
                v-for="(line, i) in visibleSamples(g)"
                :key="i"
                class="text-secondary leading-6 py-0.5"
              >{{ line }}</div>
              <el-button
                v-if="shouldShowExpand(g)"
                text
                size="small"
                class="mt-1"
                @click="toggleTemplateSamples(g.templateId)"
              >
                {{ expandedTemplates[g.templateId] ? '收起' : `展开全部 (${g.sampleLines.length} 条)` }}
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

      <!-- 日志列表（虚拟滚动） -->
      <div
        v-if="viewMode === 'flat' || templateGroups.length === 0"
        class="log-virtual-list"
      >
        <!-- 表头 -->
        <div class="log-virtual-header">
          <span class="col-index">#</span>
          <span class="col-source">来源</span>
          <span class="col-line">日志行</span>
        </div>
        <RecycleScroller
          class="log-scroller"
          :items="rows"
          :item-size="ROW_HEIGHT"
          key-field="__rowKey"
          v-slot="{ item, index }"
        >
          <div class="log-virtual-row">
            <span class="col-index text-muted">{{ index + 1 }}</span>
            <span class="col-source text-secondary">{{ item.source || '—' }}</span>
            <div class="col-line log-cell">
              <code class="whitespace-pre-wrap break-all text-xs log-cell-content">{{ item.line }}</code>
              <el-button
                :icon="MagicStick"
                size="small"
                class="log-similar-btn"
                title="语义找相似"
                @click.stop="onFindSimilar(item.line)"
              />
              <el-button
                :icon="DocumentCopy"
                size="small"
                class="log-copy-btn"
                @click.stop="handleCopy(index)"
              />
            </div>
          </div>
        </RecycleScroller>
      </div>

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

    <!-- 语义检索结果弹窗 -->
    <el-dialog
      v-model="semanticDialogVisible"
      title="语义相似检索"
      width="640px"
      :close-on-click-modal="true"
    >
      <div class="mb-3">
        <span class="text-sm text-muted">查询：</span>
        <code class="text-xs bg-muted-bg px-2 py-1 rounded font-mono break-all">{{ semanticQuery }}</code>
      </div>
      <div v-loading="semanticSearching">
        <div v-if="semanticResults.length > 0" class="space-y-2">
          <div
            v-for="(r, i) in semanticResults"
            :key="i"
            class="semantic-result-item p-3 rounded-lg border border-light hover:border-brand-300 transition-colors"
          >
            <div class="flex items-center justify-between mb-1">
              <span class="text-xs text-muted">{{ r.targetType }}: {{ r.targetId?.slice(0, 16) }}...</span>
              <el-tag size="small" type="success">相似度 {{ (r.score * 100).toFixed(1) }}%</el-tag>
            </div>
            <code class="text-xs font-mono text-secondary break-all leading-5">{{ r.metadata?.template_text || r.metadata?.sample_line || '—' }}</code>
          </div>
        </div>
        <el-empty v-else-if="!semanticSearching" description="未找到相似模板（可能需要更多日志写入以建立向量索引）" />
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
/** 历史日志检索页 v2.0：选择客户端 + 日志项，按日期和关键词跨节点检索日志。 */
import { ref, reactive, shallowRef, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useRouteQuery } from '@/composables/useRouteQuery'
import TaskProgressRing from '@/components/TaskProgressRing.vue'
import { Search, Refresh, Download, Link, VideoPlay, Share, DocumentCopy, Clock, CircleCheckFilled, Loading, MagicStick } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { RecycleScroller } from 'vue-virtual-scroller'
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css'
import { queryLogClients, queryItemsByClient, queryLogs, searchLogs, queryTaskProgress, searchSemantic, searchTemplates } from '@/api/logs'
import { queryAllItems } from '@/api/item'
import LogHistogram from '@/components/LogHistogram.vue'
import { useSearchHistory } from '@/store/searchHistory'
import SearchHistoryDropdown from '@/components/SearchHistoryDropdown.vue'

// 虚拟滚动行高（px）
const ROW_HEIGHT = 28

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const searching = ref(false)

const viewMode = ref('cluster')
// 聚类排序方式（V2.4-09）：count 按命中次数（后端默认），time 按首次出现时间
const groupSort = ref('count')
const templateGroups = ref([])
const activeTemplateGroups = ref([])
const expandedTemplates = ref({})

// 模板样本展示上限（V2.4-09）
//
// 后端 groupByTemplate 最多返回 3 条样本（与 FR-702 一致），
// 折叠态默认展示 1 条，点击「展开全部」后展示全部 3 条。
// 此前前端写死 slice(0, 5)，与后端上限不一致导致展开按钮永不出现。
const COLLAPSED_SAMPLE_LIMIT = 1

function visibleSamples(group) {
  const samples = group?.sampleLines || []
  if (expandedTemplates.value[group?.templateId]) return samples
  return samples.slice(0, COLLAPSED_SAMPLE_LIMIT)
}

function shouldShowExpand(group) {
  return (group?.sampleLines?.length || 0) > COLLAPSED_SAMPLE_LIMIT
}

function toggleTemplateSamples(templateId) {
  expandedTemplates.value = {
    ...expandedTemplates.value,
    [templateId]: !expandedTemplates.value[templateId],
  }
}

// sortedTemplateGroups 按选定方式排序后的聚类列表（V2.4-09）
const sortedTemplateGroups = computed(() => {
  const list = [...(templateGroups.value || [])]
  if (groupSort.value === 'time') {
    // 按首次出现时间倒序（新的在前）；缺失时排末尾
    return list.sort((a, b) => {
      const ta = a?.firstSeen || ''
      const tb = b?.firstSeen || ''
      if (ta === tb) return 0
      return ta < tb ? 1 : -1
    })
  }
  // 默认按命中次数降序（后端已排好，这里做稳定再排保证一致性）
  return list.sort((a, b) => (b?.count || 0) - (a?.count || 0))
})

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
const taskProgressPct = ref(0)
const taskNodeDone = ref(0)
const taskNodeTotal = ref(0)
const taskEstimatedTotal = ref(0)
const progressTimer = ref(null)
const hasMore = ref(false)
const loadingMore = ref(false)
const router = useRouter()

// 语义检索弹窗状态
const semanticDialogVisible = ref(false)
const semanticSearching = ref(false)
const semanticQuery = ref('')
const semanticResults = ref([])

// 模板自动补全状态
const templateSuggestions = ref([])
const templateSuggestionsVisible = ref(false)
const templateSearchTimer = ref(null)

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
    templateSuggestionsVisible.value = false
    if (historyDropdownRef.value) historyDropdownRef.value.hide()
  }, 150)
}

// onTemplateInput 模板自动补全输入处理（防抖 300ms）。
function onTemplateInput() {
  if (templateSearchTimer.value) clearTimeout(templateSearchTimer.value)
  const kw = form.key?.trim() || ''
  if (kw.length < 2) {
    templateSuggestions.value = []
    templateSuggestionsVisible.value = false
    return
  }
  templateSearchTimer.value = setTimeout(async () => {
    try {
      const { data } = await searchTemplates(kw, 8)
      if (data && data.code === 0 && data.data?.list?.length > 0) {
        templateSuggestions.value = data.data.list
        templateSuggestionsVisible.value = true
      } else {
        templateSuggestions.value = []
        templateSuggestionsVisible.value = false
      }
    } catch {
      templateSuggestions.value = []
      templateSuggestionsVisible.value = false
    }
  }, 300)
}

// selectTemplateSuggestion 选中模板建议，填入关键词并搜索。
function selectTemplateSuggestion(tpl) {
  form.key = tpl.templateText || tpl.sampleLine || form.key
  templateSuggestionsVisible.value = false
  onSearch()
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

// 启动任务进度轮询（每 1.5s 请求一次轻量进度接口）
function startProgressPolling(taskNo) {
  stopProgressPolling()
  if (!taskNo) return
  taskProgressPct.value = 0
  taskNodeDone.value = 0
  taskEstimatedTotal.value = 0
  progressTimer.value = setInterval(async () => {
    try {
      const { data: resp } = await queryTaskProgress(taskNo)
      const d = resp.data || {}
      if (typeof d.progressPct === 'number') taskProgressPct.value = Math.min(100, d.progressPct)
      if (typeof d.nodeDone === 'number') taskNodeDone.value = d.nodeDone
      if (typeof d.nodeTotal === 'number') taskNodeTotal.value = d.nodeTotal
      if (typeof d.estimatedTotal === 'number') taskEstimatedTotal.value = d.estimatedTotal
      if (d.status === 'success' || d.status === 'failed') stopProgressPolling()
    } catch {
      // 轮询失败不阻塞搜索，静默忽略
    }
  }, 1500)
}

function stopProgressPolling() {
  if (progressTimer.value) {
    clearInterval(progressTimer.value)
    progressTimer.value = null
  }
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

// 语义检索：基于向量相似度查找相似日志模板（FR-601）。
async function onFindSimilar(line) {
  if (!line) return
  semanticQuery.value = line
  semanticResults.value = []
  semanticDialogVisible.value = true
  semanticSearching.value = true
  try {
    const { data } = await searchSemantic({ query: line, type: 'template', topK: 10 })
    if (data && data.code === 0) {
      semanticResults.value = data.data?.results || []
    } else {
      ElMessage.error(data?.msg || '语义检索失败')
    }
  } catch {
    ElMessage.error('语义检索请求失败')
  } finally {
    semanticSearching.value = false
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
  taskProgressPct.value = 0
  taskNodeDone.value = 0
  taskNodeTotal.value = 0
  taskEstimatedTotal.value = 0
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
    if (payload.code && payload.code !== 0) {
      ElMessage.error(payload.msg || '检索失败')
      return
    }
    const data = payload.data || {}
    rows.value = (data.list || []).map((row, i) => ({
      ...row,
      __rowKey: `${Date.now()}_${i}_${row.line?.slice(0, 20) || ''}`,
    }))
    resultTotal.value = data.total || 0
    resultTruncated.value = !!data.truncated
    hasMore.value = data.total > pageSize.value * pageNo.value
    currentTaskNo.value = data.taskNo || ''
    if (currentTaskNo.value && data.nodeTotal > 1) {
      startProgressPolling(currentTaskNo.value)
    }
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
    stopProgressPolling()
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
      const newRows = (data.list || []).map((row, i) => ({
        ...row,
        __rowKey: `${Date.now()}_${rows.value.length + i}_${row.line?.slice(0, 20) || ''}`,
      }))
      rows.value = rows.value.concat(newRows)
    } else {
      rows.value = (data.list || []).map((row, i) => ({
        ...row,
        __rowKey: `${Date.now()}_${i}_${row.line?.slice(0, 20) || ''}`,
      }))
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

// 消费全局搜索跳转携带的 query（V2.1-07）：
//   ?item=12&key=timeout   → 回填表单
//   ?item=12&query=level:ERROR → 回填结构化查询
// 无 query 时不改变用户当前表单状态。
const { itemID, keyword, queryExpr } = useRouteQuery()

onMounted(() => {
  loadClients()
  loadItems()
  searchStore.load()

  if (itemID.value) form.item = itemID.value
  if (queryExpr.value) {
    form.query = queryExpr.value
    form.key = ''
  } else if (keyword.value) {
    form.key = keyword.value
  }
})

onUnmounted(() => {
  stopProgressPolling()
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
.log-copy-btn,
.log-similar-btn {
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.2s;
  padding: 4px;
}
.log-cell:hover .log-copy-btn,
.log-cell:hover .log-similar-btn { opacity: 1; }
.log-similar-btn { color: var(--brand-primary); }
.log-cell-content { flex: 1; }

/* ===== 虚拟滚动日志列表 ===== */
.log-virtual-list {
  border: 1px solid var(--border-light);
  border-radius: var(--radius-md);
  overflow: hidden;
}
.log-virtual-header {
  display: flex;
  align-items: center;
  height: 36px;
  padding: 0 12px;
  background: var(--bg-card-muted);
  border-bottom: 1px solid var(--border-light);
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-secondary);
}
.log-virtual-header .col-index { width: 55px; flex-shrink: 0; }
.log-virtual-header .col-source { width: 160px; flex-shrink: 0; }
.log-virtual-header .col-line { flex: 1; }
.log-scroller {
  height: 480px;
}
.log-virtual-row {
  display: flex;
  align-items: center;
  height: 28px;
  padding: 0 12px;
  border-bottom: 1px solid var(--border-light);
  font-size: 12.5px;
  line-height: 28px;
  overflow: hidden;
}
.log-virtual-row:hover {
  background: var(--bg-card-hover);
}
.log-virtual-row .col-index { width: 55px; flex-shrink: 0; }
.log-virtual-row .col-source {
  width: 160px;
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.log-virtual-row .col-line { flex: 1; min-width: 0; overflow: hidden; }

/* 语义检索结果 */
.semantic-result-item {
  background: var(--bg-card-muted);
}
.semantic-result-item:hover {
  border-color: var(--brand-primary) !important;
}

/* 模板自动补全 */
.template-autocomplete__item:hover {
  background: var(--bg-card-hover);
}
.template-autocomplete__item:hover code {
  color: var(--brand-primary);
}

/* 背景色 */
.bg-muted-bg {
  background: var(--bg-card-muted);
}
</style>
