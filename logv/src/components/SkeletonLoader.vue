<template>
  <div class="skeleton-loader" role="status" aria-busy="true" aria-label="加载中" data-testid="skeleton-loader">
    <!-- table 类型：模拟表格行 -->
    <div v-if="type === 'table'" class="space-y-3">
      <div
        v-for="i in count"
        :key="'row-' + i"
        class="flex items-center gap-3"
      >
        <div class="skeleton-line h-4 w-1/4 rounded bg-gray-200 dark:bg-gray-300 animate-pulse" />
        <div class="skeleton-line h-4 w-1/3 rounded bg-gray-200 dark:bg-gray-300 animate-pulse" style="animation-delay: 0.1s" />
        <div class="skeleton-line h-4 w-1/6 rounded bg-gray-200 dark:bg-gray-300 animate-pulse" style="animation-delay: 0.2s" />
        <div class="skeleton-line h-4 w-1/5 rounded bg-gray-200 dark:bg-gray-300 animate-pulse" style="animation-delay: 0.3s" />
      </div>
    </div>

    <!-- card 类型：模拟卡片网格 -->
    <div v-else-if="type === 'card'" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
      <div
        v-for="i in count"
        :key="'card-' + i"
        class="space-y-3 rounded-xl bg-white dark:bg-gray-800 p-4 border border-gray-100 dark:border-gray-700"
      >
        <div class="skeleton-line h-5 w-2/3 rounded bg-gray-200 dark:bg-gray-700 animate-pulse" />
        <div class="skeleton-line h-3 w-full rounded bg-gray-200 dark:bg-gray-700 animate-pulse" style="animation-delay: 0.1s" />
        <div class="skeleton-line h-3 w-5/6 rounded bg-gray-200 dark:bg-gray-700 animate-pulse" style="animation-delay: 0.2s" />
      </div>
    </div>

    <!-- text 类型：模拟文本段落 -->
    <div v-else-if="type === 'text'" class="space-y-2">
      <div
        v-for="i in count"
        :key="'text-' + i"
        class="skeleton-line h-3 rounded bg-gray-200 dark:bg-gray-300 animate-pulse"
        :style="{ width: (100 - (i * 8)) + '%', animationDelay: (i * 0.08) + 's' }"
      />
    </div>

    <!-- circle 类型：模拟圆形头像/图标 -->
    <div v-else-if="type === 'circle'" class="flex items-center gap-4">
      <div
        v-for="i in count"
        :key="'circle-' + i"
        class="skeleton-line rounded-full bg-gray-200 dark:bg-gray-300 animate-pulse"
        :style="{ width: '40px', height: '40px', animationDelay: (i * 0.1) + 's' }"
      />
    </div>

    <!-- chart 类型：模拟图表区域 -->
    <div v-else-if="type === 'chart'" class="space-y-3">
      <!-- 图表区域模拟 -->
      <div class="skeleton-box h-48 w-full rounded-lg bg-gray-200 dark:bg-gray-300 animate-pulse" />
      <!-- 图例模拟 -->
      <div class="flex items-center gap-4">
        <div
          v-for="i in 3"
          :key="'legend-' + i"
          class="skeleton-line h-3 rounded bg-gray-200 dark:bg-gray-300 animate-pulse"
          :style="{ width: (60 + i * 30) + 'px', animationDelay: (i * 0.15) + 's' }"
        />
      </div>
    </div>

    <!-- 默认回退：text 类型 -->
    <div v-else class="space-y-2">
      <div
        v-for="i in count"
        :key="'default-' + i"
        class="skeleton-line h-3 rounded bg-gray-200 dark:bg-gray-300 animate-pulse"
        :style="{ width: (100 - (i * 8)) + '%', animationDelay: (i * 0.08) + 's' }"
      />
    </div>
  </div>
</template>

<script setup>
/**
 * SkeletonLoader — 通用骨架屏组件
 *
 * 使用 Tailwind CSS 的 animate-pulse + 灰阶背景实现脉动加载效果，
 * 支持五种骨架形态，主题友好（dark 模式自动适配）。
 */
defineProps({
  /** 骨架屏类型：table | card | text | circle | chart */
  type: {
    type: String,
    default: 'text',
    validator: (val) => ['table', 'card', 'text', 'circle', 'chart'].includes(val),
  },
  /** 骨架条目数量 */
  count: {
    type: Number,
    default: 3,
    validator: (val) => val >= 1 && val <= 20,
  },
})
</script>
