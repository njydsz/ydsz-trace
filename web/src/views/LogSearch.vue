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
          <el-input v-model="form.key" placeholder="要检索的关键字" />
        </el-form-item>
        <el-form-item label="上下文行数">
          <el-input-number v-model="form.line" :min="0" :max="500" />
          <span class="text-gray-400 text-sm ml-2">命中行上下的上下文行数</span>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :icon="Search" :loading="searching" @click="onSearch">
            开始检索
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

/** 日志检索页：选择客户端 + 日志项，按日期和关键词跨节点检索日志。结果以 zip 下载，错误以 JSON 返回还原。 */
<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { queryLogClients, queryItemsByClient, queryLogs } from '@/api/logs'
import { queryAllItems } from '@/api/item'

const clients = ref([])
const items = ref([])
const itemsLoading = ref(false)
const searching = ref(false)
const form = reactive({ client: 0, item: null, date: '', key: '', line: 20 })

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
  } catch {
    ElMessage.error('检索请求失败，请确认服务端已启动')
  } finally {
    searching.value = false
  }
}

onMounted(() => {
  loadClients()
  loadItems()
})
</script>
