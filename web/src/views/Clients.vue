<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl font-semibold">客户端管理</h2>
      <el-button type="primary" :icon="Plus" @click="openAdd">新增客户端</el-button>
    </div>

    <el-card shadow="never">
      <el-table :data="list" v-loading="loading" stripe border>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="ip" label="IP" />
        <el-table-column prop="port" label="端口" width="90" />
        <el-table-column prop="vkey" label="密钥" width="120" />
        <el-table-column prop="info" label="备注" show-overflow-tooltip />
        <el-table-column label="在线状态" width="110">
          <template #default="{ row }">
            <el-tag :type="row.online === '1' ? 'success' : 'info'">
              {{ row.online === '1' ? '在线' : '离线' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用状态" width="110">
          <template #default="{ row }">
            <el-tag :type="row.status === '1' ? 'success' : 'danger'">
              {{ row.status === '1' ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdTime" label="创建时间" width="180" />
        <el-table-column label="操作" width="220" fixed="right">
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

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑客户端' : '新增客户端'" width="480px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="IP">
          <el-input v-model="form.ip" placeholder="客户端 IP" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input v-model="form.port" placeholder="默认 2020" />
        </el-form-item>
        <el-form-item label="密钥">
          <el-input v-model="form.vkey" placeholder="与 logc 配置一致" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.info" />
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
import { ref, reactive, onMounted } from 'vue'
import { Plus, Edit, Delete, Switch } from '@element-plus/icons-vue'
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
  await changeClientStatus(row.id)
  ElMessage.success('状态已更新')
  load()
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认删除客户端 ${row.ip}:${row.port} ？`, '提示', { type: 'warning' })
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
