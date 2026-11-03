<template>
  <div class="task-progress-ring inline-flex items-center justify-center">
    <svg
      :width="size"
      :height="size"
      :viewBox="`0 0 ${size} ${size}`"
      role="progressbar"
      :aria-valuenow="clampedPercent"
      aria-valuemin="0"
      :aria-valuemax="100"
      :aria-label="`任务进度 ${clampedPercent}%`"
    >
      <!-- 背景圆环 -->
      <circle
        :cx="center"
        :cy="center"
        :r="radius"
        :stroke="trackColor"
        :stroke-width="strokeWidth"
        fill="none"
      />
      <!-- 进度圆弧 -->
      <circle
        :cx="center"
        :cy="center"
        :r="radius"
        :stroke="color"
        :stroke-width="strokeWidth"
        fill="none"
        :stroke-dasharray="circumference"
        :stroke-dashoffset="dashOffset"
        stroke-linecap="round"
        class="progress-arc transition-all duration-500 ease-out"
        :transform="`rotate(-90 ${center} ${center})`"
      />
    </svg>
    <!-- 中心文字 -->
    <div class="progress-label" :style="{ fontSize: size * 0.2 + 'px', color }">
      {{ clampedPercent }}%
    </div>
  </div>
</template>

<script setup>
/**
 * TaskProgressRing — SVG 圆环进度条组件
 *
 * 可视化展示任务进度，使用 SVG stroke-dasharray 实现圆弧填充效果。
 * 具备完整可访问性属性（role + aria-value*）。
 */
import { computed } from 'vue'

const props = defineProps({
  /** 进度百分比（0-100） */
  percent: {
    type: Number,
    default: 0,
    validator: (val) => val >= 0 && val <= 100,
  },
  /** 圆环直径（像素） */
  size: {
    type: Number,
    default: 80,
    validator: (val) => val >= 24 && val <= 512,
  },
  /** 进度条颜色 */
  color: {
    type: String,
    default: '#2563eb',
  },
})

// 限制 percent 在 0-100
const clampedPercent = computed(() => Math.max(0, Math.min(100, props.percent)))

// 几何计算
const strokeWidth = computed(() => Math.max(props.size * 0.08, 3))
const center = computed(() => props.size / 2)
const radius = computed(() => (props.size - strokeWidth.value) / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)

// 进度偏移量：100% → 0（满圆），0% → circumference（空圆）
const dashOffset = computed(() => {
  const progress = clampedPercent.value / 100
  return circumference.value * (1 - progress)
})

// 轨道颜色：半透明的灰色
const trackColor = 'rgba(156, 163, 175, 0.25)'
</script>

<style scoped>
.task-progress-ring {
  position: relative;
}

.progress-label {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  font-weight: 600;
  line-height: 1;
  user-select: none;
}
</style>
