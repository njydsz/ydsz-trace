<template>
  <div class="clients-page">
    <!-- 顶部操作栏 -->
    <div class="page-header mb-5">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="page-title flex items-center gap-2">
            <div class="page-icon flex items-center justify-center rounded-lg">
              <el-icon :size="18"><Cpu /></el-icon>
            </div>
            客户端配置
          </h2>
          <p class="page-subtitle mt-1">管理所有已注册的日志采集客户端节点</p>
        </div>
        <el-button type="primary" :icon="Plus" size="default" @click="openAdd" class="action-btn">
          新增客户端
        </el-button>
      </div>
    </div>

    <!-- 内容卡片 -->
    <el-card shadow="never" class="!rounded-xl">
      <!-- 搜索框 -->
      <div class="toolbar flex items-center gap-3 mb-4">
        <el-input
          v-model="searchText"
          placeholder="搜索名称、IP、端口、来源..."
          :debounce="200"
          clearable
          :prefix-icon="Search"
          class="search-input"
          style="max-width: 320px"
        />
        <transition name="fade">
          <div v-if="selectedRows.length > 0" class="batch-actions flex items-center gap-2 pl-3 border-l" style="border-color: var(--border-light)">
            <span class="text-sm text-muted">已选 <strong class="text-primary">{{ selectedRows.length }}</strong> 项</span>
            <el-button type="success" size="small" :icon="Check" @click="handleBatchEnable">启用</el-button>
            <el-button type="warning" size="small" :icon="Close" @click="handleBatchDisable">禁用</el-button>
            <el-button type="danger" size="small" :icon="Delete" @click="handleBatchDelete">删除</el-button>
          </div>
        </transition>
      </div>

      <el-skeleton :rows="6" animated v-if="loading && list.length === 0" />
      <el-table
        v-if="!(loading && list.length === 0)"
        :data="filteredList"
        v-loading="loading"
        stripe
        border
        @selection-change="handleSelectionChange"
        ref="tableRef"
        class="clients-table"
      >
        <el-table-column type="selection" width="48" />
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="名称" min-width="120" show-overflow-tooltip />
        <el-table-column prop="ip" label="IP 地址" width="140">
          <template #default="{ row }">
            <code class="ip-code">{{ row.ip }}</code>
          </template>
        </el-table-column>
        <el-table-column prop="port" label="端口" width="80" align="center" />
        <el-table-column prop="source" label="来源" width="100" show-overflow-tooltip />
        <el-table-column label="密钥" width="120">
          <template #default="{ row }">
            <span v-if="row.vkey" class="vkey-mask">{{ maskSecret(row.vkey) }}</span>
            <span v-else class="text-muted text-xs">未设置</span>
          </template>
        </el-table-column>
        <el-table-column prop="info" label="备注" min-width="120" show-overflow-tooltip />
        <el-table-column label="在线" width="90" align="center">
          <template #default="{ row }">
            <span class="status-dot" :class="row.online === '1' ? 'status-dot--online' : 'status-dot--offline'" />
            <span class="status-text">{{ row.online === '1' ? '在线' : '离线' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <span class="status-badge" :class="row.status === '1' ? 'status-badge--active' : 'status-badge--inactive'">
              {{ row.status === '1' ? '启用' : '禁用' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="createdTime" label="创建时间" width="170" />
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" :icon="Edit" @click="openEdit(row)">编辑</el-button>
            <el-button link size="small" :icon="Switch" @click="toggleStatus(row)">{{ row.status === '1' ? '禁用' : '启用' }}</el-button>
            <el-button link type="danger" size="small" :icon="Delete" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="flex justify-end mt-4">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="limit"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          background
          @current-change="load"
          @size-change="load"
        />
      </div>
    </el-card>

    <!-- 新增/编辑 dialog -->
    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑客户端' : '新增客户端'" width="500px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="IP" required>
          <el-input v-model="form.ip" placeholder="客户端 IP 地址" />
        </el-form-item>
        <el-form-item label="端口" required>
          <el-input v-model="form.port" placeholder="默认 2020" />
        </el-form-item>
        <el-form-item label="密钥">
          <el-input v-model="form.vkey" placeholder="与 logc 配置一致" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.info" placeholder="备注信息（可选）" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch
            v-model="statusOn"
            active-value="1"
            inactive-value="0"
            active-text="启用"
            inactive-text="禁用"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
/** 客户端配置页 v2.0：列表展示、增删改查、启用/禁用切换。搜索过滤、批量操作。删除前弹确认框。 */
import { ref, reactive, onMounted, computed } from 'vue'
import { Plus, Edit, Delete, Switch, Search, Check, Close } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  queryClients,
  addClient,
  updateClient,
  deleteClient,
  changeClientStatus,
} from '@/api/client'

const list = ref([])
const total = ref(0)
const page = ref(1)
const limit = ref(10)
const loading = ref(false)
const searchText = ref('')
const selectedRows = ref([])
const tableRef = ref(null)

function maskSecret(vkey) {
  if (!vkey) return ''
  return '******'
}

function filterClients(clients, keyword) {
  const kw = keyword.trim().toLowerCase()
  if (!kw) return clients
  return clients.filter(
    (c) =>
      (c.name || '').toLowerCase().includes(kw) ||
      (c.ip || '').toLowerCase().includes(kw) ||
      String(c.port || '').includes(kw) ||
      (c.source || '').toLowerCase().includes(kw)
  )
}

const filteredList = computed(() => filterClients(list.value, searchText.value))

function handleSelectionChange(selection) {
  selectedRows.value = selection
}

async function handleBatchEnable() {
  const ids = selectedRows.value.map((r) => r.id)
  loading.value = true
  try {
    await Promise.all(ids.map((id) => changeClientStatus(id, 1)))
    ElMessage.success(`已启用 ${ids.length} 个客户端`)
    searchText.value = ''
    await load()
    selectedRows.value = []
    tableRef.value?.clearSelection()
  } finally {
    loading.value = false
  }
}

async function handleBatchDisable() {
  const ids = selectedRows.value.map((r) => r.id)
  loading.value = true
  try {
    await Promise.all(ids.map((id) => changeClientStatus(id, 0)))
    ElMessage.success(`已禁用 ${ids.length} 个客户端`)
    searchText.value = ''
    await load()
    selectedRows.value = []
    tableRef.value?.clearSelection()
  } finally {
    loading.value = false
  }
}

async function handleBatchDelete() {
  const ids = selectedRows.value.map((r) => r.id)
  try {
    await ElMessageBox.confirm(`确认删除所选 ${ids.length} 个客户端？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  loading.value = true
  try {
    await Promise.all(ids.map((id) => deleteClient(id)))
    ElMessage.success(`已删除 ${ids.length} 个客户端`)
    searchText.value = ''
    await load()
    selectedRows.value = []
    tableRef.value?.clearSelection()
  } finally {
    loading.value = false
  }
}

const dialogVisible = ref(false)
const isEdit = ref(false)
const saving = ref(false)
const statusOn = ref('1')
const form = reactive({ id: 0, ip: '', port: '2020', vkey: '', info: '', status: '1' })

async function load() {
  loading.value = true
  try {
    const { data } = await queryClients(page.value, limit.value)
    if (data.code === '200' && data.data) {
      list.value = data.data.list || []
      total.value = data.data.totalCount || 0
    }
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.id = 0
  form.ip = ''
  form.port = '2020'
  form.vkey = ''
  form.info = ''
  form.status = '1'
  statusOn.value = '1'
}

function openAdd() {
  isEdit.value = false
  resetForm()
  dialogVisible.value = true
}

function openEdit(row) {
  isEdit.value = true
  form.id = row.id
  form.ip = row.ip
  form.port = row.port
  form.vkey = row.vkey
  form.info = row.info
  form.status = row.status || '1'
  statusOn.value = form.status
  dialogVisible.value = true
}

async function save() {
  if (!form.ip || !form.port) {
    ElMessage.warning('请填写 IP 和端口')
    return
  }
  saving.value = true
  form.status = statusOn.value
  try {
    const payload = { ...form }
    const { data } = isEdit.value ? await updateClient(payload) : await addClient(payload)
    if (data.code === '200') {
      ElMessage.success(data.msg)
      dialogVisible.value = false
      load()
    } else {
      ElMessage.error(data.msg || '保存失败')
    }
  } finally {
    saving.value = false
  }
}

async function toggleStatus(row) {
  const newStatus = row.status === '1' ? 0 : 1
  await changeClientStatus(row.id, newStatus)
  ElMessage.success('状态已更新')
  load()
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认删除客户端 ${row.ip}:${row.port}？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  const { data } = await deleteClient(row.id)
  if (data.code === '200') {
    ElMessage.success('删除成功')
    load()
  } else {
    ElMessage.error(data.msg || '删除失败')
  }
}

onMounted(load)
</script>

<style scoped>
/* ===== 页面标题区 ===== */
.page-header {
  position: relative;
}
.page-title {
  font-size: 20px;
  font-weight: 700;
  color: var(--text-primary);
  margin: 0;
}
.page-icon {
  width: 36px;
  height: 36px;
  background: linear-gradient(135deg, var(--brand-500), var(--brand-700));
  color: white;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.2);
}
.page-subtitle {
  font-size: 13px;
  color: var(--text-muted);
  margin: 0;
}
.action-btn {
  font-weight: 500 !important;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.2);
}

/* ===== 工具栏 ===== */
.toolbar {
  flex-wrap: wrap;
}

/* ===== 表格增强 ===== */
.clients-table :deep(.el-table__header th) {
  font-size: 12.5px !important;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.ip-code {
  font-size: 12px;
  color: var(--brand-700);
  background: var(--brand-50);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: 'SF Mono', 'Monaco', 'Consolas', monospace;
}
.vkey-mask {
  font-family: monospace;
  color: var(--text-muted);
  font-size: 12px;
}

/* ===== 状态指示 ===== */
.status-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  margin-right: 5px;
  vertical-align: middle;
}
.status-dot--online {
  background: var(--success);
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.15);
}
.status-dot--offline {
  background: var(--neutral-400);
}
.status-text {
  font-size: 12.5px;
  color: var(--text-secondary);
  vertical-align: middle;
}

.status-badge {
  display: inline-block;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  border-radius: var(--radius-full);
  line-height: 18px;
}
.status-badge--active {
  background: var(--success-light);
  color: #059669;
}
.status-badge--inactive {
  background: var(--danger-light);
  color: #dc2626;
}

/* 批量操作 */
.batch-actions {
  animation: slideIn 0.2s ease;
}
@keyframes slideIn {
  from { opacity: 0; transform: translateX(-8px); }
  to { opacity: 1; transform: translateX(0); }
}

/* 过渡 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
