<template>
  <div>
    <h2 class="text-xl font-semibold mb-4">告警</h2>

    <el-tabs v-model="activeTab">
      <!-- 配额摘要 -->
      <div class="flex items-center gap-4 mb-4">
        <el-tag type="primary">启用规则: {{ quota.enabled }}</el-tag>
        <el-tag type="success">近 24h 触发: {{ quota.firedToday }}</el-tag>
        <el-tag type="danger">近 24h 失败: {{ quota.failedToday }}</el-tag>
        <el-button :icon="Refresh" @click="loadQuota" size="small">刷新摘要</el-button>
      </div>

      <el-tab-pane label="告警规则" name="rules">
        <div class="flex items-center gap-3 mb-3">
          <el-select v-model="ruleForm.itemId" placeholder="选择日志项" style="width: 240px" :loading="itemsLoading">
            <el-option v-for="it in items" :key="it.id" :label="it.itemName" :value="it.id" />
          </el-select>
          <el-button type="primary" :icon="Plus" @click="onAddRule">新增规则</el-button>
          <el-button :icon="Refresh" @click="loadRules">刷新</el-button>
        </div>

        <el-table :data="rules" v-loading="loading" stripe border>
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column prop="name" label="名称" min-width="140" />
          <el-table-column label="日志项" min-width="140" show-overflow-tooltip>
            <template #default="{ row }">
              <span>{{ row.itemName || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="匹配条件" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <span>{{ ruleSummary(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="阈值" width="80" align="center">
            <template #default="{ row }">
              <span>≥ {{ row.threshold }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="intervalSec" label="评估间隔" width="100" align="center">
            <template #default="{ row }"><span>{{ row.intervalSec }}s</span></template>
          </el-table-column>
          <el-table-column prop="lastFired" label="最近触发" width="170" />
          <el-table-column label="启用" width="90">
            <template #default="{ row }">
              <el-switch
                :model-value="row.enabled === 1"
                @change="onToggle(row)"
              />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="220" fixed="right">
            <template #default="{ row }">
              <el-button link :icon="Promotion" size="small" type="primary" @click="onTestFire(row)">测试触发</el-button>
              <el-button link :icon="Delete" size="small" type="danger" @click="onDeleteRule(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="mt-3 flex justify-end">
          <el-pagination
            v-model:current-page="pageNo"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50]"
            :total="total"
            layout="total, sizes, prev, pager, next"
            background
            @current-change="loadRules"
            @size-change="loadRules"
          />
        </div>
      </el-tab-pane>

      <el-tab-pane label="投递记录" name="events">
        <div class="flex items-center gap-3 mb-3">
          <el-select v-model="eventStatusFilter" placeholder="全部状态" clearable style="width: 140px" @change="loadEvents">
            <el-option label="成功" value="ok" />
            <el-option label="失败" value="fail" />
          </el-select>
          <el-button :icon="Refresh" @click="loadEvents">刷新</el-button>
        </div>

        <el-table :data="events" v-loading="eventsLoading" stripe border>
          <el-table-column prop="id" label="ID" width="70" />
          <el-table-column prop="ruleName" label="规则" min-width="140" />
          <el-table-column prop="matchCount" label="命中" width="90" align="center" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" effect="light" round>
                {{ row.status === 'ok' ? '成功' : '失败' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="httpCode" label="HTTP" width="90" align="center" />
          <el-table-column prop="sampleText" label="样本" min-width="220" show-overflow-tooltip />
          <el-table-column prop="errorMsg" label="错误" min-width="180" show-overflow-tooltip />
          <el-table-column prop="firedTime" label="触发时间" width="170" />
          <el-table-column label="操作" width="90" fixed="right">
            <template #default="{ row }">
              <el-button link size="small" type="danger" :icon="Delete" @click="onDeleteEvent(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="mt-3 flex justify-end">
          <el-pagination
            v-model:current-page="eventPageNo"
            v-model:page-size="eventPageSize"
            :page-sizes="[10, 20, 50]"
            :total="eventTotal"
            layout="total, sizes, prev, pager, next"
            background
            @current-change="loadEvents"
            @size-change="loadEvents"
          />
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 新增/编辑规则 dialog -->
    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑规则' : '新增规则'" width="560px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="如: ERROR 过多告警" />
        </el-form-item>
        <el-form-item label="日志项" required>
          <el-select v-model="form.itemId" placeholder="选择日志项" style="width: 100%">
            <el-option v-for="it in items" :key="it.id" :label="it.itemName" :value="it.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="客户端">
          <el-select v-model="form.clientId" placeholder="全部客户端" clearable style="width: 100%">
            <el-option label="全部客户端（聚合）" :value="0" />
            <el-option v-for="c in clients" :key="c.id" :label="`${c.ip}:${c.port}`" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="关键词">
          <el-input v-model="form.keyWord" placeholder="匹配的关键字或正则表达式" />
        </el-form-item>
        <el-form-item label="正则模式">
          <el-switch v-model="form.regex" active-text="正则" inactive-text="包含" />
        </el-form-item>
        <el-form-item label="日志级别">
          <el-select v-model="form.level" placeholder="不过滤" clearable style="width: 100%">
            <el-option v-for="lv in ['DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL']" :key="lv" :label="lv" :value="lv" />
          </el-select>
        </el-form-item>
        <el-form-item label="触发阈值">
          <el-input-number v-model="form.threshold" :min="1" :max="10000" />
          <span class="text-gray-400 text-sm ml-2">命中达到该数量触发</span>
        </el-form-item>
        <el-form-item label="评估间隔(秒)">
          <el-input-number v-model="form.intervalSec" :min="60" :max="86400" />
        </el-form-item>
        <el-form-item label="Webhook" required>
          <el-input v-model="form.webhookUrl" placeholder="https://your-webhook/hook" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="onSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
/**
 * 告警页：规则管理 (CRUD / toggle / test-fire) + 投递事件历史。
 */
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Plus, Promotion, Delete } from '@element-plus/icons-vue'
import {
  listRules, addRule, updateRule, deleteRule, toggleRule, testFireRule,
  listEvents, deleteEvent, getQuota,
} from '@/api/alert'

const activeTab = ref('rules')
const rules = ref([])
const events = ref([])
const items = ref([])
const clients = ref([])
const quota = ref({ enabled: 0, firedToday: 0, failedToday: 0 })

const loading = ref(false)
const eventsLoading = ref(false)
const itemsLoading = ref(false)

const pageNo = ref(1)
const pageSize = ref(20)
const total = ref(0)
const eventPageNo = ref(1)
const eventPageSize = ref(20)
const eventTotal = ref(0)
const eventStatusFilter = ref('')

const dialogVisible = ref(false)
const editingId = ref(null)

const ruleForm = reactive({ itemId: null })
const form = reactive({
  name: '',
  itemId: null,
  clientId: 0,
  keyWord: '',
  regex: false,
  level: '',
  threshold: 10,
  intervalSec: 300,
  webhookUrl: '',
})

async function loadRules() {
  loading.value = true
  try {
    const { data } = await listRules({ pageNo: pageNo.value, pageSize: pageSize.value })
    if (data && data.code === 0) {
      const p = data.data || {}
      rules.value = p.list || []
      total.value = p.totalCount || 0
    }
  } finally {
    loading.value = false
  }
}

async function loadEvents() {
  eventsLoading.value = true
  try {
    const { data } = await listEvents({
      pageNo: eventPageNo.value,
      pageSize: eventPageSize.value,
      status: eventStatusFilter.value,
    })
    if (data && data.code === 0) {
      const p = data.data || {}
      events.value = p.list || []
      eventTotal.value = p.totalCount || 0
    }
  } finally {
    eventsLoading.value = false
  }
}

async function loadQuota() {
  const { data } = await getQuota()
  if (data && data.code === 0) {
    quota.value = data.data || { enabled: 0, firedToday: 0, failedToday: 0 }
  }
}

function ruleSummary(r) {
  const parts = []
  if (r.keyWord) parts.push(r.regex ? `[正则] ${r.keyWord}` : r.keyWord)
  if (r.level) parts.push(`[${r.level}]`)
  return parts.length ? parts.join(' ') : '—'
}

function onAddRule() {
  editingId.value = null
  Object.assign(form, {
    name: '', itemId: ruleForm.itemId || null, clientId: 0,
    keyWord: '', regex: false, level: '',
    threshold: 10, intervalSec: 300, webhookUrl: '',
  })
  dialogVisible.value = true
}

async function onSubmit() {
  if (!form.name) return ElMessage.warning('请输入规则名称')
  if (!form.itemId) return ElMessage.warning('请选择日志项')
  if (!form.webhookUrl) return ElMessage.warning('请输入 webhook URL')
  try {
    if (editingId.value) {
      await updateRule(editingId.value, { ...form })
      ElMessage.success('更新成功')
    } else {
      await addRule({ ...form })
      ElMessage.success('新增成功')
    }
    dialogVisible.value = false
    loadRules()
  } catch {
    ElMessage.error('操作失败')
  }
}

async function onToggle(row) {
  const next = row.enabled === 1 ? 0 : 1
  await toggleRule(row.id, next)
  row.enabled = next
  ElMessage.success(next ? '已启用' : '已禁用')
}

async function onTestFire(row) {
  try {
    await ElMessageBox.confirm(`手动触发规则 ${row.name} 一次`, '测试触发', { type: 'info' })
  } catch {
    return
  }
  await testFireRule(row.id)
  ElMessage.success('已提交测试，请稍后查看投递记录')
}

async function onDeleteRule(row) {
  try {
    await ElMessageBox.confirm(`确认删除规则 ${row.name}？`, '删除规则', { type: 'warning' })
  } catch {
    return
  }
  await deleteRule(row.id)
  ElMessage.success('删除成功')
  loadRules()
}

async function onDeleteEvent(row) {
  try {
    await ElMessageBox.confirm('确认删除该投递记录？', '删除', { type: 'warning' })
  } catch {
    return
  }
  await deleteEvent(row.id)
  ElMessage.success('删除成功')
  loadEvents()
}

onMounted(() => {
  loadRules()
  loadEvents()
  loadQuota()
})
</script>
