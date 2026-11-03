<template>
  <div class="ai-status-panel">
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-sm font-semibold text-primary flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 2a4 4 0 0 1 4 4v1a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2h-8a2 2 0 0 1-2-2v-2a2 2 0 0 1 2-2V6a4 4 0 0 1 4-4z"/>
          <path d="M8 14v4"/><path d="M16 14v4"/>
        </svg>
        AI 四层智能引擎
      </h3>
      <span class="text-xs text-muted">近 24h {{ total24h }} 次调用</span>
    </div>

    <div v-if="loading" class="flex items-center justify-center py-4">
      <el-icon class="animate-spin text-brand-500"><Loading /></el-icon>
    </div>

    <div v-else class="space-y-2">
      <div
        v-for="layer in layers"
        :key="layer.layer"
        class="layer-row flex items-center justify-between p-2 rounded-lg transition-colors hover:bg-muted-bg"
      >
        <div class="flex items-center gap-3">
          <span
            class="status-dot w-2 h-2 rounded-full"
            :class="{
              'bg-green-400': layer.status === 'active',
              'bg-yellow-400': layer.status === 'degraded',
              'bg-gray-400': layer.status === 'inactive'
            }"
          />
          <div>
            <span class="text-sm font-medium text-primary">{{ layer.name }}</span>
            <p class="text-xs text-muted leading-tight mt-0.5">{{ layer.description }}</p>
            <div v-if="layer.algorithms && layer.algorithms.length" class="flex flex-wrap gap-1 mt-1">
              <span
                v-for="alg in layer.algorithms"
                :key="alg"
                class="text-[10px] px-1.5 py-0.5 rounded bg-muted-bg text-muted"
              >{{ alg }}</span>
            </div>
          </div>
        </div>
        <div class="text-right">
          <el-tag
            size="small"
            :type="layer.status === 'active' ? 'success' : layer.status === 'degraded' ? 'warning' : 'info'"
          >
            {{ layer.status === 'active' ? '运行中' : layer.status === 'degraded' ? '降级' : '未激活' }}
          </el-tag>
          <div v-if="layer.model" class="text-xs text-muted mt-1">{{ layer.model }}</div>
          <div v-if="layer.callCount24H > 0" class="text-xs text-muted mt-0.5">
            {{ layer.callCount24H }} 次
          </div>
        </div>
      </div>
    </div>

    <div v-if="generatedAt" class="mt-3 text-xs text-muted text-right">
      更新于 {{ generatedAt }}
    </div>
  </div>
</template>

<script setup>
/**
 * AIStatusPanel 展示 L0-L3 智能漏斗各层运行状态。
 *
 * 数据源: GET /admin/ai/status
 * 层说明:
 *   L0 rule: Drain 模板匹配（始终 active）
 *   L1 stats: 基线 + 突变检测（有数据时 active）
 *   L2 embedding: 向量化 + 余弦相似度（当前未实现 HNSW，算法以 algorithms 字段为准）
 *   L3 generation: NL2DSL 规则解析（Reasoning agent / 根因推理待 V2.3）
 */
import { ref, onMounted } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { fetchAIStatus } from '@/api/aimodel'

const loading = ref(false)
const layers = ref([])
const total24h = ref(0)
const generatedAt = ref('')

onMounted(async () => {
  loading.value = true
  try {
    const { data } = await fetchAIStatus()
    if (data && data.code === 0 && data.data) {
      layers.value = data.data.layers || []
      total24h.value = data.data.total24H || 0
      generatedAt.value = data.data.generatedAt || ''
    }
  } catch {
    layers.value = []
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.ai-status-panel {
  background: var(--bg-card);
  border: 1px solid var(--border-card);
  border-radius: 12px;
  padding: 16px;
}
.layer-row {
  border: 1px solid transparent;
}
</style>
