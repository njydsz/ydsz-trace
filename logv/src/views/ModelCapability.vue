<template>
  <div>
    <!-- 统一页面头部 -->
    <div class="page-header mb-5">
      <div class="flex items-center justify-between">
        <div>
          <h2 class="page-header__title">
            <span class="page-header__icon" style="background: linear-gradient(135deg, #8b5cf6, #7c3aed); box-shadow: 0 2px 8px rgba(139, 92, 246, 0.2);">
              <el-icon :size="18"><MagicStick /></el-icon>
            </span>
            模型能力配置
          </h2>
          <p class="page-header__subtitle">管理 AI 模型接入参数，支持对话(chat)、向量嵌入(embedding)、重排序(rerank) 三类能力</p>
        </div>
        <el-button type="primary" :icon="Plus" @click="openAdd">新增配置</el-button>
      </div>
    </div>

    <el-card shadow="never" class="!rounded-xl">
      <!-- 过滤区 -->
      <div class="flex items-center gap-3 mb-4 flex-wrap">
        <el-input
          v-model="searchText"
          placeholder="搜索名称、模型、提供商"
          :debounce="200"
          clearable
          :prefix-icon="Search"
          style="max-width: 280px"
        />
        <el-radio-group v-model="capabilityFilter" size="default" @change="onCapabilityFilterChange">
          <el-radio-button label="">全部</el-radio-button>
          <el-radio-button label="chat">对话</el-radio-button>
          <el-radio-button label="embedding">嵌入</el-radio-button>
          <el-radio-button label="rerank">重排序</el-radio-button>
        </el-radio-group>
      </div>

      <el-skeleton :rows="6" animated v-if="loading && list.length === 0" />
      <el-table
        v-if="!(loading && list.length === 0)"
        :data="list"
        v-loading="loading"
        stripe
        border
      >
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="配置名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="provider" label="提供商" width="110">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ providerLabel(row.provider) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="modelId" label="模型 ID" min-width="160" show-overflow-tooltip />
        <el-table-column label="能力类型" width="100">
          <template #default="{ row }">
            <el-tag :type="capabilityTagType(row.capability)" size="small">
              {{ capabilityLabel(row.capability) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="temperature" label="温度" width="70" align="center" />
        <el-table-column prop="maxTokens" label="最大 Token" width="100" align="center">
          <template #default="{ row }">{{ formatMaxTokens(row.maxTokens) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled === 1"
              @change="toggleStatus(row)"
              :active-value="1"
              :inactive-value="0"
              inline-prompt
              active-text="开"
              inactive-text="关"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Edit" @click="openEdit(row)">编辑</el-button>
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

    <!-- 新增/编辑弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑模型能力配置' : '新增模型能力配置'"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="100px">
        <el-form-item label="配置名称" prop="name">
          <el-input v-model="form.name" placeholder="如：GPT-4o、本地 Embedding" maxlength="64" show-word-limit />
        </el-form-item>
        <el-form-item label="提供商" prop="provider">
          <el-select v-model="form.provider" placeholder="选择提供商" style="width: 100%">
            <el-option label="OpenAI" value="openai" />
            <el-option label="Anthropic" value="anthropic" />
            <el-option label="Ollama (本地)" value="ollama" />
            <el-option label="自定义" value="custom" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型 ID" prop="modelId">
          <el-input v-model="form.modelId" placeholder="如 gpt-4o, claude-3-sonnet" />
        </el-form-item>
        <el-form-item label="能力类型" prop="capability">
          <el-radio-group v-model="form.capability">
            <el-radio value="chat">对话</el-radio>
            <el-radio value="embedding">向量嵌入</el-radio>
            <el-radio value="rerank">重排序</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="API 密钥" prop="apiKey">
          <el-input
            v-model="form.apiKey"
            :type="apiKeyVisible ? 'text' : 'password'"
            placeholder="留空表示不修改（编辑时）"
            :show-password="true"
          >
            <template #suffix>
              <el-icon class="cursor-pointer" @click="apiKeyVisible = !apiKeyVisible">
                <View v-if="apiKeyVisible" /><Hide v-else />
              </el-icon>
            </template>
          </el-input>
          <div class="form-hint">密钥将以加密形式存储</div>
        </el-form-item>
        <el-form-item label="API 端点" prop="baseUrl">
          <el-input v-model="form.baseUrl" placeholder="留空使用提供商默认端点" />
        </el-form-item>
        <el-form-item label="采样温度" prop="temperature">
          <el-slider v-model="form.temperature" :min="0" :max="2" :step="0.1" show-input />
        </el-form-item>
        <el-form-item label="最大 Token" prop="maxTokens">
          <el-input-number v-model="form.maxTokens" :min="256" :max="128000" :step="512" />
        </el-form-item>
        <el-form-item label="状态" prop="enabled">
          <el-switch
            v-model="form.enabled"
            :active-value="1"
            :inactive-value="0"
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
/**
 * 模型能力配置页：管理 AI 模型(LLM/Embedding) 的接入参数。
 * 支持 chat/embedding/rerank 三类能力的配置 CRUD 与启停切换。
 */
import { ref, reactive, onMounted } from 'vue'
import {
  Plus, Edit, Delete, Search, MagicStick, View, Hide,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listModelCapabilities,
  addModelCapability,
  updateModelCapability,
  deleteModelCapability,
  toggleModelCapability,
} from '@/api/aimodel'

const list = ref([])
const total = ref(0)
const page = ref(1)
const limit = ref(10)
const loading = ref(false)
const searchText = ref('')
const capabilityFilter = ref('')

const dialogVisible = ref(false)
const isEdit = ref(false)
const saving = ref(false)
const formRef = ref(null)
const apiKeyVisible = ref(false)

const form = reactive({
  id: 0,
  name: '',
  provider: 'openai',
  modelId: '',
  apiKey: '',
  baseUrl: '',
  temperature: 0.7,
  maxTokens: 4096,
  enabled: 1,
  capability: 'chat',
})

const formRules = {
  name: [
    { required: true, message: '请输入配置名称', trigger: 'blur' },
    { min: 2, max: 64, message: '长度在 2~64 字符之间', trigger: 'blur' },
  ],
  provider: [{ required: true, message: '请选择提供商', trigger: 'change' }],
  modelId: [
    { required: true, message: '请输入模型 ID', trigger: 'blur' },
  ],
  capability: [{ required: true, message: '请选择能力类型', trigger: 'change' }],
  temperature: [
    { type: 'number', min: 0, max: 2, message: '温度取值范围 0~2', trigger: 'change' },
  ],
  maxTokens: [
    { type: 'number', min: 256, max: 128000, message: 'Token 数取值范围 256~128000', trigger: 'change' },
  ],
}

// providerLabel 将提供商英文标识转为中文显示。
function providerLabel(provider) {
  const map = { openai: 'OpenAI', anthropic: 'Anthropic', ollama: 'Ollama', custom: '自定义' }
  return map[provider] || provider
}

// capabilityLabel 显示能力类型中文。
function capabilityLabel(cap) {
  const map = { chat: '对话', embedding: '嵌入', rerank: '重排序' }
  return map[cap] || cap
}

// capabilityTagType 返回 Element Plus tag 的主题类型。
function capabilityTagType(cap) {
  const map = { chat: 'primary', embedding: 'success', rerank: 'warning' }
  return map[cap] || 'info'
}

// formatMaxTokens 格式化 Token 数显示。
function formatMaxTokens(v) {
  if (!v) return '-'
  if (v >= 1000) return (v / 1000).toFixed(0) + 'K'
  return String(v)
}

// onCapabilityFilterChange 能力类型筛选变更后重新加载第一页。
function onCapabilityFilterChange() {
  page.value = 1
  load()
}

async function load() {
  loading.value = true
  try {
    const params = { pageNo: page.value, pageSize: limit.value }
    if (searchText.value.trim()) params.keyword = searchText.value.trim()
    if (capabilityFilter.value) params.capability = capabilityFilter.value
    const { data } = await listModelCapabilities(params)
    if (data.code === '200' && data.data) {
      list.value = data.data.list || []
      total.value = data.data.total || 0
    }
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.id = 0
  form.name = ''
  form.provider = 'openai'
  form.modelId = ''
  form.apiKey = ''
  form.baseUrl = ''
  form.temperature = 0.7
  form.maxTokens = 4096
  form.enabled = 1
  form.capability = 'chat'
  apiKeyVisible.value = false
}

function openAdd() {
  isEdit.value = false
  resetForm()
  dialogVisible.value = true
}

function openEdit(row) {
  isEdit.value = true
  form.id = row.id
  form.name = row.name || ''
  form.provider = row.provider || 'openai'
  form.modelId = row.modelId || ''
  form.apiKey = ''
  form.baseUrl = row.baseUrl || ''
  form.temperature = row.temperature ?? 0.7
  form.maxTokens = row.maxTokens || 4096
  form.enabled = row.enabled ?? 1
  form.capability = row.capability || 'chat'
  apiKeyVisible.value = false
  dialogVisible.value = true
}

async function save() {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    const payload = { ...form }
    // 编辑时 apiKey 为空表示不更新
    if (isEdit.value && !payload.apiKey) {
      delete payload.apiKey
    }
    const apiFn = isEdit.value ? updateModelCapability : addModelCapability
    const { data } = await apiFn(payload)
    if (data.code === '200' || data.code === '0' || data.code === 0) {
      ElMessage.success(data.message || '保存成功')
      dialogVisible.value = false
      load()
    } else {
      ElMessage.error(data.message || data.msg || '保存失败')
    }
  } finally {
    saving.value = false
  }
}

async function toggleStatus(row) {
  const newEnabled = row.enabled === 1 ? 0 : 1
  try {
    const { data } = await toggleModelCapability(row.id, newEnabled)
    if (data.code === '200' || data.code === '0' || data.code === 0) {
      ElMessage.success(newEnabled === 1 ? '已启用' : '已禁用')
      row.enabled = newEnabled
    } else {
      ElMessage.error(data.message || data.msg || '操作失败')
    }
  } catch {
    /* 失败时 UI 自动回滚（switch 受 model-value 控制） */
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认删除模型能力配置「${row.name}」？删除后关联的 AI 功能将不可用。`, '确认删除', { type: 'warning' })
  } catch {
    return
  }
  const { data } = await deleteModelCapability(row.id)
  if (data.code === '200' || data.code === '0' || data.code === 0) {
    ElMessage.success('删除成功')
    load()
  } else {
    ElMessage.error(data.message || data.msg || '删除失败')
  }
}

onMounted(load)
</script>

<style scoped>
.page-header__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  color: white;
  margin-right: 10px;
  vertical-align: middle;
}
.page-header__title {
  font-size: 18px;
  font-weight: 700;
  margin: 0;
  display: flex;
  align-items: center;
}
.page-header__subtitle {
  font-size: 13px;
  color: var(--text-muted);
  margin: 4px 0 0 46px;
}
.form-hint {
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.5;
}
.cursor-pointer {
  cursor: pointer;
}
</style>
