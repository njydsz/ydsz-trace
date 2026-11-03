<template>
  <div>
    <!-- 统一页面头部 -->
    <div class="page-header mb-5">
      <div class="flex items-center justify-between flex-wrap gap-3">
        <div>
          <h2 class="page-header__title">
            <span class="page-header__icon" style="background: linear-gradient(135deg, #8b5cf6, #7c3aed); box-shadow: 0 2px 8px rgba(139, 92, 246, 0.2);">
              <svg :width="18" :height="18" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/>
              </svg>
            </span>
            模型能力配置
          </h2>
          <p class="page-header__subtitle">管理 AI 模型接入参数，支持对话(chat)、向量嵌入(embedding)、重排序(rerank) 三类能力</p>
        </div>
        <button
          class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg bg-brand-600 text-white text-sm font-medium hover:bg-brand-700 active:bg-brand-800 transition-colors shadow-sm"
          @click="openAdd"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
          </svg>
          新增配置
        </button>
      </div>
    </div>

    <!-- 主卡片 -->
    <div class="rounded-xl bg-white shadow-card border border-gray-200/60 p-5">
      <!-- 过滤区 -->
      <div class="flex items-center gap-3 mb-4 flex-wrap">
        <div class="relative" style="max-width: 280px; width: 100%">
          <div class="absolute inset-y-0 left-0 flex items-center pl-3 pointer-events-none">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-gray-400">
              <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
            </svg>
          </div>
          <input
            v-model="searchText"
            type="text"
            placeholder="搜索名称、模型、提供商"
            class="w-full pl-9 pr-8 py-2 text-sm border border-gray-200 rounded-lg bg-white placeholder:text-gray-400 focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
          />
          <button
            v-if="searchText"
            class="absolute inset-y-0 right-0 flex items-center pr-2.5 text-gray-400 hover:text-gray-600"
            @click="searchText = ''"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>
        <div class="inline-flex rounded-lg border border-gray-200 overflow-hidden">
          <button
            v-for="opt in [{label:'全部',value:''},{label:'对话',value:'chat'},{label:'嵌入',value:'embedding'},{label:'重排序',value:'rerank'}]"
            :key="opt.value"
            :class="[
              'px-3 py-1.5 text-sm font-medium transition-colors',
              capabilityFilter === opt.value
                ? 'bg-brand-600 text-white'
                : 'bg-white text-gray-600 hover:bg-gray-50'
            ]"
            @click="capabilityFilter = opt.value; onCapabilityFilterChange()"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <!-- 骨架屏 -->
      <div v-if="loading && list.length === 0" class="space-y-3">
        <div v-for="i in 6" :key="i" class="flex items-center gap-3 animate-pulse">
          <div class="h-4 w-1/8 rounded bg-gray-200" />
          <div class="h-4 w-1/4 rounded bg-gray-200" />
          <div class="h-4 w-1/6 rounded bg-gray-200" />
          <div class="h-4 w-1/6 rounded bg-gray-200" />
          <div class="h-4 w-1/12 rounded bg-gray-200" />
          <div class="h-4 w-1/12 rounded bg-gray-200" />
        </div>
      </div>

      <!-- 数据表格 -->
      <div v-if="!(loading && list.length === 0)" class="overflow-x-auto">
        <table v-if="list.length > 0" class="w-full text-sm">
          <thead>
            <tr class="border-b border-gray-100 bg-gray-50/60">
              <th class="px-3 py-2.5 text-left font-semibold text-xs text-gray-500 uppercase tracking-wide w-[70px]">ID</th>
              <th class="px-3 py-2.5 text-left font-semibold text-xs text-gray-500 uppercase tracking-wide">配置名称</th>
              <th class="px-3 py-2.5 text-left font-semibold text-xs text-gray-500 uppercase tracking-wide w-[110px]">提供商</th>
              <th class="px-3 py-2.5 text-left font-semibold text-xs text-gray-500 uppercase tracking-wide">模型 ID</th>
              <th class="px-3 py-2.5 text-left font-semibold text-xs text-gray-500 uppercase tracking-wide w-[100px]">能力类型</th>
              <th class="px-3 py-2.5 text-center font-semibold text-xs text-gray-500 uppercase tracking-wide w-[70px]">温度</th>
              <th class="px-3 py-2.5 text-center font-semibold text-xs text-gray-500 uppercase tracking-wide w-[100px]">最大 Token</th>
              <th class="px-3 py-2.5 text-center font-semibold text-xs text-gray-500 uppercase tracking-wide w-[80px]">状态</th>
              <th class="px-3 py-2.5 text-center font-semibold text-xs text-gray-500 uppercase tracking-wide w-[90px]">连通性</th>
              <th class="px-3 py-2.5 text-center font-semibold text-xs text-gray-500 uppercase tracking-wide w-[160px]">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(row, idx) in list"
              :key="row.id"
              class="border-b border-gray-50 hover:bg-brand-50/30 transition-colors"
              :class="idx % 2 === 1 ? 'bg-gray-50/30' : ''"
            >
              <td class="px-3 py-2.5 text-gray-600">{{ row.id }}</td>
              <td class="px-3 py-2.5 text-gray-900 font-medium max-w-[140px] truncate" :title="row.name">{{ row.name }}</td>
              <td class="px-3 py-2.5">
                <span class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700">
                  {{ providerLabel(row.provider) }}
                </span>
              </td>
              <td class="px-3 py-2.5 text-gray-600 font-mono text-xs max-w-[160px] truncate" :title="row.modelId">{{ row.modelId }}</td>
              <td class="px-3 py-2.5">
                <span :class="capabilityTagClass(row.capability)" class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">
                  {{ capabilityLabel(row.capability) }}
                </span>
              </td>
              <td class="px-3 py-2.5 text-center text-gray-600">{{ row.temperature }}</td>
              <td class="px-3 py-2.5 text-center text-gray-600">{{ formatMaxTokens(row.maxTokens) }}</td>
              <td class="px-3 py-2.5 text-center">
                <button
                  :class="[
                    'relative inline-flex h-5 w-9 items-center rounded-full transition-colors',
                    row.enabled === 1 ? 'bg-brand-600' : 'bg-gray-300'
                  ]"
                  @click="toggleStatus(row)"
                >
                  <span
                    :class="[
                      'inline-block h-3.5 w-3.5 rounded-full bg-white shadow transition-transform',
                      row.enabled === 1 ? 'translate-x-4.5' : 'translate-x-1'
                    ]"
                  />
                </button>
              </td>
              <td class="px-3 py-2.5 text-center">
                <button
                  :class="[
                    'text-xs font-medium transition-colors',
                    row._selfCheckStatus === 'ok' ? 'text-green-600 hover:text-green-700' :
                    row._selfCheckStatus === 'fail' ? 'text-red-600 hover:text-red-700' :
                    'text-gray-500 hover:text-brand-600'
                  ]"
                  :disabled="row._selfChecking"
                  @click="handleSelfCheck(row)"
                >
                  <span v-if="row._selfChecking" class="inline-flex items-center gap-1">
                    <svg class="animate-spin h-3 w-3" viewBox="0 0 24 24" fill="none">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                    </svg>
                    检测中
                  </span>
                  <span v-else>{{ selfCheckLabel(row) }}</span>
                </button>
              </td>
              <td class="px-3 py-2.5 text-center">
                <button class="text-brand-600 hover:text-brand-700 text-xs font-medium mr-2" @click="openEdit(row)">编辑</button>
                <button class="text-red-600 hover:text-red-700 text-xs font-medium" @click="remove(row)">删除</button>
              </td>
            </tr>
          </tbody>
        </table>

        <!-- 空状态 -->
        <div v-if="list.length === 0 && !loading" class="text-center py-12">
          <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"/>
          </svg>
          <p class="mt-2 text-sm text-gray-500">暂无模型能力配置</p>
        </div>
      </div>

      <!-- 分页 -->
      <div v-if="list.length > 0" class="flex items-center justify-between mt-4 flex-wrap gap-3">
        <div class="flex items-center gap-2 text-sm text-gray-500">
          <span>共 {{ total }} 条</span>
          <select
            :value="limit"
            class="border border-gray-200 rounded px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500"
            @change="limit = Number($event.target.value); page = 1; load()"
          >
            <option :value="10">10 条/页</option>
            <option :value="20">20 条/页</option>
            <option :value="50">50 条/页</option>
          </select>
        </div>
        <div class="flex items-center gap-1">
          <button
            :disabled="page <= 1"
            class="px-2 py-1 text-sm border border-gray-200 rounded hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            @click="page--; load()"
          >
            上一页
          </button>
          <span class="px-3 py-1 text-sm text-gray-600">
            {{ page }} / {{ Math.max(1, Math.ceil(total / limit)) }}
          </span>
          <button
            :disabled="page >= Math.ceil(total / limit)"
            class="px-2 py-1 text-sm border border-gray-200 rounded hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            @click="page++; load()"
          >
            下一页
          </button>
        </div>
      </div>
    </div>

    <!-- 新增/编辑模态框 -->
    <Teleport to="body">
      <div v-if="dialogVisible" class="fixed inset-0 z-[2000] flex items-center justify-center">
        <div class="absolute inset-0 bg-black/40" @click="dialogVisible = false" />
        <div class="relative bg-white rounded-2xl shadow-xl w-[560px] max-w-[90vw] max-h-[85vh] overflow-y-auto">
          <!-- 模态头部 -->
          <div class="flex items-center justify-between px-6 py-4 border-b border-gray-100">
            <h3 class="text-base font-semibold text-gray-900">
              {{ isEdit ? '编辑模型能力配置' : '新增模型能力配置' }}
            </h3>
            <button class="text-gray-400 hover:text-gray-600 transition-colors" @click="dialogVisible = false">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
            </button>
          </div>
          <!-- 表单区域 -->
          <div class="px-6 py-5 space-y-4">
            <!-- 配置名称 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-1">配置名称</label>
              <input
                v-model="form.name"
                type="text"
                placeholder="如：GPT-4o、本地 Embedding"
                maxlength="64"
                class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
              />
              <div class="form-hint">长度 2~64 字符</div>
            </div>
            <!-- 提供商 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-1">提供商</label>
              <select
                v-model="form.provider"
                class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
              >
                <option value="openai">OpenAI</option>
                <option value="anthropic">Anthropic</option>
                <option value="ollama">Ollama (本地)</option>
                <option value="custom">自定义</option>
              </select>
            </div>
            <!-- 模型 ID -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-1">模型 ID</label>
              <input
                v-model="form.modelId"
                type="text"
                placeholder="如 gpt-4o, claude-3-sonnet"
                class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
              />
            </div>
            <!-- 能力类型 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-2">能力类型</label>
              <div class="flex gap-4">
                <label v-for="cap in [{v:'chat',l:'对话'},{v:'embedding',l:'向量嵌入'},{v:'rerank',l:'重排序'}]" :key="cap.v" class="flex items-center gap-1.5 cursor-pointer">
                  <input type="radio" v-model="form.capability" :value="cap.v" class="accent-brand-600" />
                  <span class="text-sm text-gray-700">{{ cap.l }}</span>
                </label>
              </div>
            </div>
            <!-- API 密钥 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-1">API 密钥</label>
              <div class="relative">
                <input
                  v-model="form.apiKey"
                  :type="apiKeyVisible ? 'text' : 'password'"
                  placeholder="留空表示不修改（编辑时）"
                  class="w-full px-3 py-2 pr-10 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
                />
                <button
                  type="button"
                  class="absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400 hover:text-gray-600"
                  @click="apiKeyVisible = !apiKeyVisible"
                >
                  <svg v-if="apiKeyVisible" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/>
                    <line x1="1" y1="1" x2="23" y2="23"/>
                  </svg>
                  <svg v-else width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>
                  </svg>
                </button>
              </div>
              <div class="form-hint">密钥将以加密形式存储</div>
            </div>
            <!-- API 端点 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-1">API 端点</label>
              <input
                v-model="form.baseUrl"
                type="text"
                placeholder="留空使用提供商默认端点"
                class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
              />
            </div>
            <!-- 采样温度 -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-1">采样温度: {{ form.temperature }}</label>
              <input
                v-model.number="form.temperature"
                type="range"
                min="0"
                max="2"
                step="0.1"
                class="w-full accent-brand-600"
              />
              <div class="flex justify-between text-xs text-gray-400">
                <span>0</span><span>1</span><span>2</span>
              </div>
            </div>
            <!-- 最大 Token -->
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-1">最大 Token</label>
              <div class="flex items-center gap-2">
                <button
                  class="w-8 h-8 flex items-center justify-center border border-gray-200 rounded text-gray-600 hover:bg-gray-50 transition-colors"
                  @click="form.maxTokens = Math.max(256, form.maxTokens - 512)"
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="5" y1="12" x2="19" y2="12"/></svg>
                </button>
                <input
                  v-model.number="form.maxTokens"
                  type="number"
                  min="256"
                  max="128000"
                  step="512"
                  class="flex-1 px-3 py-2 text-sm border border-gray-200 rounded-lg text-center focus:outline-none focus:ring-2 focus:ring-brand-500/20 focus:border-brand-500 transition-all"
                />
                <button
                  class="w-8 h-8 flex items-center justify-center border border-gray-200 rounded text-gray-600 hover:bg-gray-50 transition-colors"
                  @click="form.maxTokens = Math.min(128000, form.maxTokens + 512)"
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
                </button>
              </div>
            </div>
            <!-- 状态 -->
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700">状态</label>
              <button
                :class="[
                  'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                  form.enabled === 1 ? 'bg-brand-600' : 'bg-gray-300'
                ]"
                @click="form.enabled = form.enabled === 1 ? 0 : 1"
              >
                <span
                  :class="[
                    'inline-block h-4 w-4 rounded-full bg-white shadow transition-transform',
                    form.enabled === 1 ? 'translate-x-5' : 'translate-x-1'
                  ]"
                />
              </button>
            </div>
          </div>
          <!-- 模态底部 -->
          <div class="flex items-center justify-end gap-3 px-6 py-4 border-t border-gray-100">
            <button
              class="px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200 transition-colors"
              @click="dialogVisible = false"
            >
              取消
            </button>
            <button
              class="inline-flex items-center gap-1.5 px-4 py-2 text-sm font-medium text-white bg-brand-600 rounded-lg hover:bg-brand-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              :disabled="saving"
              @click="save"
            >
              <svg v-if="saving" class="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
              </svg>
              保存
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup>
/**
 * 模型能力配置页：管理 AI 模型(LLM/Embedding) 的接入参数。
 * 支持 chat/embedding/rerank 三类能力的配置 CRUD 与启停切换。
 */
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listModelCapabilities,
  addModelCapability,
  updateModelCapability,
  deleteModelCapability,
  toggleModelCapability,
  selfCheckModelCapability,
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

// capabilityTagClass 返回能力类型对应的 Tailwind 色块样式。
function capabilityTagClass(cap) {
  const map = {
    chat: 'bg-brand-100 text-brand-700 dark:bg-brand-900/30 dark:text-brand-300',
    embedding: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
    rerank: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  }
  return map[cap] || 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'
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
    if (data.code === 0 && data.data) {
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
  // 简单前端校验
  if (!form.name || form.name.length < 2 || form.name.length > 64) {
    ElMessage.warning('请输入 2~64 字符的配置名称')
    return
  }
  if (!form.provider) {
    ElMessage.warning('请选择提供商')
    return
  }
  if (!form.modelId) {
    ElMessage.warning('请输入模型 ID')
    return
  }
  if (!form.capability) {
    ElMessage.warning('请选择能力类型')
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
    if (data.code === 0) {
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
    if (data.code === 0) {
      ElMessage.success(newEnabled === 1 ? '已启用' : '已禁用')
      row.enabled = newEnabled
    } else {
      ElMessage.error(data.message || data.msg || '操作失败')
    }
  } catch {
    /* 失败时 UI 手动回滚，通过 v-bind 重新渲染 */
    row.enabled = row.enabled === 1 ? 1 : 0
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认删除模型能力配置「${row.name}」？删除后关联的 AI 功能将不可用。`, '确认删除', { type: 'warning' })
  } catch {
    return
  }
  const { data } = await deleteModelCapability(row.id)
  if (data.code === 0) {
    ElMessage.success('删除成功')
    load()
  } else {
    ElMessage.error(data.message || data.msg || '删除失败')
  }
}

// selfCheckLabel 根据自检状态返回按钮文本。
function selfCheckLabel(row) {
  if (row._selfChecking) return '检测中'
  if (row._selfCheckStatus === 'ok') return '✓ 可达'
  if (row._selfCheckStatus === 'fail') return '✗ 不可达'
  return '自检'
}

// handleSelfCheck 执行连通性自检并展示结果提示。
async function handleSelfCheck(row) {
  row._selfChecking = true
  try {
    const { data } = await selfCheckModelCapability(row.id)
    if (data.code === 0 && data.data) {
      const reachable = data.data.reachable
      row._selfCheckStatus = reachable ? 'ok' : 'fail'
      const latency = data.data.latencyMs !== undefined && data.data.latencyMs !== null ? ` (${data.data.latencyMs}ms)` : ''
      if (reachable) {
        ElMessage.success(`模型「${row.name}」可达${latency}：${data.data.message}`)
      } else {
        ElMessage.error(`模型「${row.name}」不可达：${data.data.message}`)
      }
    } else {
      row._selfCheckStatus = 'fail'
      ElMessage.error(data.message || '自检失败')
    }
  } catch (err) {
    row._selfCheckStatus = 'fail'
    ElMessage.error('自检请求失败：' + (err?.message || err))
  } finally {
    row._selfChecking = false
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
  margin-top: 4px;
}
</style>
