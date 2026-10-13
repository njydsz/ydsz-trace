<template>
  <el-card shadow="never" class="query-result-card">
    <div class="flex items-center justify-between mb-2">
      <span class="text-sm font-medium text-gray-700 dark:text-gray-200">
        {{ title }}
      </span>
      <span v-if="subtitle" class="text-xs text-gray-400">{{ subtitle }}</span>
    </div>
    <div class="text-2xl font-bold text-primary">
      {{ count }}
    </div>
    <div v-if="details" class="mt-2 text-xs text-gray-500">
      {{ details }}
    </div>
  </el-card>
</template>

<script setup>
/**
 * 查询结果统计卡片：用于在助手气泡中展示命中行数、过滤条件等概要信息。
 */
defineProps({
  title: { type: String, default: '命中行数' },
  subtitle: { type: String, default: '' },
  count: { type: [Number, String], required: true },
  details: { type: String, default: '' },
})
</script>

<style scoped>
.query-result-card {
  border-radius: 8px;
}
</style>
