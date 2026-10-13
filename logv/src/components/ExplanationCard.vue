<template>
  <div class="flex flex-wrap gap-1 mt-1">
    <el-tag size="small" type="info">理解: "{{ userIntent }}"</el-tag>
    <el-tag size="small">条件: {{ dslConditions }}</el-tag>
    <el-tag size="small" type="success">匹配: {{ matchCount }} 行</el-tag>
  </div>
</template>

<script setup>
/**
 * FR-501 自然语言转 DSL 解释卡。
 * 将用户自然语言意图翻译为后端检索条件，以标签可视化"理解→条件→结果"。
 */
defineProps({
  userIntent: { type: String, required: true },
  dslConditions: { type: String, required: true },
  matchCount: { type: Number, required: true },
})
</script>
