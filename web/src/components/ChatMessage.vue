<template>
  <div class="flex" :class="role === 'user' ? 'justify-end' : 'justify-start'">
    <div class="max-w-[75%] flex flex-col" :class="role === 'user' ? 'items-end' : 'items-start'">
      <!-- 消息气泡 -->
      <div
        class="px-4 py-2.5 rounded-2xl text-sm leading-relaxed break-words"
        :class="bubbleClass"
      >
        <!-- 文字内容 -->
        <p v-if="type === 'text'" class="whitespace-pre-wrap m-0">{{ content }}</p>

        <!-- 图表/卡片内容 -->
        <div v-else-if="type === 'chart'" class="space-y-3">
          <p v-if="content" class="whitespace-pre-wrap m-0">{{ content }}</p>
          <slot name="chart" />
        </div>

        <!-- 错误内容 -->
        <div v-else-if="type === 'error'" class="space-y-2">
          <el-alert type="error" :closable="false" show-icon>
            <template #title>
              {{ errorTitle }}
            </template>
            {{ errorDetail }}
          </el-alert>
          <slot name="fallback" />
        </div>
      </div>

      <!-- 时间戳（仅用户消息） -->
      <span v-if="role === &apos;user&apos;" class="text-xs text-gray-400 mt-1 px-1">
        {{ timestamp }}
      </span>

      <!-- 插槽：助手消息底部挂载解释卡等 -->
      <slot name="footer" />
    </div>
  </div>
</template>

<script setup>
/**
 * 聊天消息气泡：区分用户/助手角色、文字/图表/错误类型。
 * 用户消息靠右蓝色气泡带时间戳；助手消息靠左灰色气泡可挂载底部插槽。
 */
import { computed } from 'vue'

const props = defineProps({
  role: { type: String, required: true, validator: v => ['user', 'assistant'].includes(v) },
  type: { type: String, default: 'text', validator: v => ['text', 'chart', 'error'].includes(v) },
  content: { type: String, default: '' },
  timestamp: { type: String, default: '' },
  errorTitle: { type: String, default: '查询失败' },
  errorDetail: { type: String, default: '请尝试调整关键词或检查服务端连接' },
})

const bubbleClass = computed(() => {
  if (props.role === 'user') {
    return 'bg-blue-500 text-white rounded-br-md'
  }
  return 'bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-100 rounded-bl-md'
})
</script>
