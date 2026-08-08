<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-semibold">日志项管理</h2>
      <el-button type="primary" :icon="Plus" @click="openAdd">新增日志项</el-button>
    </div>

    <el-card shadow="never">
      <el-table :data="list" v-loading="loading" stripe border>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="itemName" label="名称" />
        <el-table-column prop="itemDesc" label="描述" show-overflow-tooltip />
        <el-table-column label="客户端" width="140">
          <template #default="{ row }">{{ clientName(row.clientId) }}</template>
        </el-table-column>
        <el-table-column prop="logPath" label="日志路径" show-overflow-tooltip />
        <el-table-column prop="logPrefix" label="前缀" width="100" />
        <el-table-column prop="logSuffix" label="后缀" width="100" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.status === '1' ? 'success' : 'danger'">
              {{ row.status === '1' ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Edit" @click="openEdit(row)">编辑</el-button>
            <el-button link type="warning" :icon="Switch" @click="toggleStatus(row)">启停</el-button>
            <el-button link type="danger" :icon="Delete" @click="remove(row)">删除</el-button>
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
          @current-change="load"
          @size-change="load"
        />
      </div>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑日志项' : '新增日志项'" width="520px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="所属客户端">
          <el-select v-model="form.clientId" placeholder="选择客户端" style="width: 100%">
            <el-option v-for="c in clients" :key="c.id" :label="`${c.ip}:${c.port}`" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="名称"><el-input v-model="form.itemName" /></el-form-item>
        <el-form-item label="描述"><el-input v-model="form.itemDesc" /></el-form-item>
        <el-form-item label="日志路径"><el-input v-model="form.logPath" placeholder="/var/log/app/" /></el-form-item>
        <el-form-item label="文件前缀"><el-input v-model="form.logPrefix" placeholder="app." /></el-form-item>
        <el-form-item label="文件后缀"><el-input v-model="form.logSuffix" placeholder=".log" /></el-form-item>
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

/** 日志项管理页：按客户端分组管理日志项，增删改查、启用/禁用切换。绑定所属客户端必填。 */
<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Plus, Edit, Delete, Switch } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { queryItems, addItem, updateItem, deleteItem, changeItemStatus } from '@/api/item'
import { queryAllClients } from '@/api/client'

const list = ref([])
const clients = ref([])
const clientMap = ref({})
const total = ref(0)
const page = ref(1)
const limit = ref(10)
const loading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const saving = ref(false)
const statusOn = ref('1')
const form = reactive({
  id: 0,
  clientId: null,
  itemName: '',
  itemDesc: '',
  logPath: '',
  logPrefix: '',
  logSuffix: '',
  status: '1',
})

function clientName(id) {
  return clientMap.value[id] || id
}

async function loadClients() {
  const { data } = await queryAllClients()
  const arr = Array.isArray(data) ? data : data.data || []
  clients.value = arr
  arr.forEach((c) => {
    clientMap.value[c.id] = `${c.ip}:${c.port}`
  })
}

async function load() {
  loading.value = true
  try {
    const { data } = await queryItems(page.value, limit.value)
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
  form.clientId = null
  form.itemName = ''
  form.itemDesc = ''
  form.logPath = ''
  form.logPrefix = ''
  form.logSuffix = ''
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
  form.clientId = row.clientId
  form.itemName = row.itemName
  form.itemDesc = row.itemDesc
  form.logPath = row.logPath
  form.logPrefix = row.logPrefix
  form.logSuffix = row.logSuffix
  form.status = row.status || '1'
  statusOn.value = form.status
  dialogVisible.value = true
}

async function save() {
  if (!form.clientId || !form.itemName) {
    ElMessage.warning('请填写所属客户端和名称')
    return
  }
  saving.value = true
  form.status = statusOn.value
  try {
    const payload = { ...form }
    const { data } = isEdit.value ? await updateItem(payload) : await addItem(payload)
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
  await changeItemStatus(row.id)
  ElMessage.success('状态已更新')
  load()
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认删除日志项 ${row.itemName} ？`, '提示', { type: 'warning' })
  } catch {
    return
  }
  const { data } = await deleteItem(row.id)
  if (data.code === '200') {
    ElMessage.success('删除成功')
    load()
  } else {
    ElMessage.error(data.msg || '删除失败')
  }
}

onMounted(() => {
  loadClients()
  load()
})
</script>
