<template>
  <div>
    <h2 class="text-xl font-semibold mb-4">检索任务</h2>

    <div class="flex items-center gap-3 mb-4">
      <el-select v-model="statusFilter" placeholder="全部状态" clearable style="width: 160px" @change="onFilterChange">
        <el-option label="全部状态" value="" />
        <el-option label="执行中" value="running" />
        <el-option label="成功" value="success" />
        <el-option label="失败" value="failed" />
        <el-option label="排队中" value="pending" />
        <el-option label="已清理" value="purged" />
      </el-select>
      <el-button :icon="Refresh" @click="loadTasks" :loading="loading">刷新</el-button>
      <span class="text-gray-500 text-sm" v-if="hasInflightTask">
        存在运行中任务，自动每 3 秒刷新
      </span>
    </div>

    <el-table :data="list" v-loading="loading" stripe border>
      <el-table-column prop="taskNo" label="任务编号" min-width="200" sortable />
      <el-table-column prop="itemName" label="日志项" min-width="140" show-overflow-tooltip />
      <el-table-column prop="logDate" label="日期" width="110" />
      <el-table-column label="关键词" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="text-gray-700">{{ summaryOf(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)" effect="light" round>
            {{ statusText(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="节点" width="100" align="center">
        <template #default="{ row }">
          <span>{{ row.nodeDone }}/{{ row.nodeTotal }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="matchCount" label="匹配行数" width="110" align="center" sortable />
      <el-table-column prop="maxLines" label="上限" width="90" align="center" />
      <el-table-column prop="createdTime" label="创建时间" width="170" sortable />
      <el-table-column prop="finishedTime" label="结束时间" width="170">
        <template #default="{ row }">
          <span>{{ row.finishedTime || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-button link size="small" :icon="View" @click="onViewDetail(row)">详情</el-button>
          <el-button
            link
            size="small"
            type="warning"
            :icon="RefreshRight"
            :disabled="!canRetry(row)"
            @click="onRetry(row)"
          >
            重试
          </el-button>
          <el-button link size="small" type="danger" :icon="Delete" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div class="flex justify-end mt-4">
      <el-pagination
        v-model:current-page="pageNo"
        v-model:page-size="pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="total"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @current-change="loadTasks"
        @size-change="loadTasks"
      />
    </div>

    <!-- 任务详情 dialog -->
    <el-dialog v-model="detailVisible" :title="`任务详情 ${selectedTask?.taskNo || ''}`" width="640px">
      <pre v-if="selectedTask" class="bg-slate-50 text-xs p-4 rounded overflow-auto max-h-96">{{ JSON.stringify(selectedTask, null, 2) }}</pre>
      <template #footer>
        <el-button @click="detailVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
/**
 * 检索任务列表页：
 *  - 分页、按状态过滤
 *  - 存在运行中/排队任务时每 3 秒自动刷新
 *  - 支持查看详情 / 重试 / 删除
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, RefreshRight, View, Delete } from '@element-plus/icons-vue'
import { listTasks, retryTask, deleteTask, TASK_STATUS } from '@/api/tasks'

const list = ref([])
const total = ref(0)
const pageNo = ref(1)
const pageSize = ref(20)
const statusFilter = ref('')
const loading = ref(false)

const detailVisible = ref(false)
const selectedTask = ref(null)

let timer = null

const hasInflightTask = computed(() =>
  list.value.some(
    (t) => t.status === TASK_STATUS.RUNNING || t.status === TASK_STATUS.PENDING
  )
)

async function loadTasks() {
  if (loading.value) return
  loading.value = true
  try {
    const { data } = await listTasks({
      page: pageNo.value,
      pageSize: pageSize.value,
      status: statusFilter.value,
    })
    if (data && data.code === 0) {
      const payload = data.data || {}
      list.value = payload.list || []
      total.value = payload.totalCount || 0
    }
  } catch {
    // 401 等由 http 拦截器统一处理
  } finally {
    loading.value = false
  }
}

function onFilterChange() {
  pageNo.value = 1
  loadTasks()
}

function summaryOf(row) {
  if (row.queryExpr) {
    return `[布尔] ${row.queryExpr.slice(0, 80)}`
  }
  if (row.keyWord) {
    return row.regex ? `[正则] ${row.keyWord}` : row.keyWord
  }
  return '—'
}

function statusText(s) {
  switch (s) {
    case TASK_STATUS.RUNNING:
      return '执行中'
    case TASK_STATUS.SUCCESS:
      return '成功'
    case TASK_STATUS.FAILED:
      return '失败'
    case TASK_STATUS.PENDING:
      return '排队中'
    case TASK_STATUS.PURGED:
      return '已清理'
    default:
      return s || '未知'
  }
}

function statusType(s) {
  switch (s) {
    case TASK_STATUS.RUNNING:
      return 'primary'
    case TASK_STATUS.SUCCESS:
      return 'success'
    case TASK_STATUS.FAILED:
      return 'danger'
    case TASK_STATUS.PENDING:
      return 'warning'
    default:
      return 'info'
  }
}

function canRetry(row) {
  return row.status === TASK_STATUS.FAILED || row.status === TASK_STATUS.SUCCESS
}

function onViewDetail(row) {
  selectedTask.value = row
  detailVisible.value = true
}

async function onRetry(row) {
  try {
    await ElMessageBox.confirm(
      `确认创建任务 ${row.taskNo} 的重试任务？`,
      '重试检索',
      { type: 'warning' }
    )
  } catch {
    return
  }
  const { data } = await retryTask(row.taskNo)
  if (data && data.code === 0) {
    ElMessage.success(`已创建重试任务：${data.data?.taskNo || ''}`)
    loadTasks()
  } else {
    ElMessage.warning(data?.msg || '重试失败')
  }
}

async function onDelete(row) {
  try {
    await ElMessageBox.confirm(
      `确认删除任务 ${row.taskNo}？此操作不可恢复。`,
      '删除任务',
      { type: 'warning' }
    )
  } catch {
    return
  }
  const { data } = await deleteTask(row.taskNo)
  if (data && data.code === 0) {
    ElMessage.success('删除成功')
    loadTasks()
  } else {
    ElMessage.warning(data?.msg || '删除失败')
  }
}

function startPolling() {
  stopPolling()
  timer = setInterval(() => {
    if (hasInflightTask.value) {
      loadTasks()
    } else {
      stopPolling()
    }
  }, 3000)
}

function stopPolling() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

onMounted(() => {
  loadTasks()
  startPolling()
})

onBeforeUnmount(() => {
  stopPolling()
})
</script>
