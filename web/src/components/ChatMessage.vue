<template>
  <div class="flex chat-msg-row" :class="role === 'user' ? 'justify-end' : 'justify-start'">
    <!-- 助手头像 -->
    <div v-if="role === 'assistant'" class="chat-avatar chat-assistant-avatar flex items-center justify-center rounded-full">
      <el-icon><Monitor /></el-icon>
    </div>

    <div class="max-w-[75%] flex flex-col mx-2" :class="role === 'user' ? 'items-end' : 'items-start'">
      <!-- 消息气泡 -->
      <div
        class="chat-bubble px-4 py-3 text-sm leading-relaxed break-words"
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
      <span v-if="role === 'user'" class="chat-timestamp mt-1 px-1">
        {{ timestamp }}
      </span>
      <slot name="footer" />
    </div>

    <!-- 用户头像 -->
    <div v-if="role === 'user'" class="chat-avatar chat-user-avatar flex items-center justify-center rounded-full">
      <el-icon><User /></el-icon>
    </div>
  </div>
</template>

<script setup>
/**
 * 聊天消息气泡 v2.0 — 更好的视觉体验。
 *
 * 用户消息: 右侧品牌渐变气泡。
 * 助手消息: 左侧带边框气泡 + 绿色机器人头像。
 * 包含头像区域和气泡 hover 效果。
 */
import { computed } from 'vue'
import { Monitor, User } from '@element-plus/icons-vue'

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
    return 'chat-bubble--user'
  }
  return 'chat-bubble--assistant'
})
</script>

<style scoped>
.chat-msg-row {
  margin-bottom: 4px;
}

/* 头像 */
.chat-avatar {
  width: 32px;
  height: 32px;
  flex-shrink: 0;
  font-size: 14px;
  align-self: flex-end;
}
.chat-user-avatar {
  background: linear-gradient(135deg, var(--brand-500), var(--brand-700));
  color: white;
  box-shadow: 0 2px 6px rgba(37, 99, 235, 0.2);
}
.chat-assistant-avatar {
  background: linear-gradient(135deg, #10b981, #059669);
  color: white;
  box-shadow: 0 2px 6px rgba(16, 185, 129, 0.2);
}

/* 气泡 */
.chat-bubble {
  box-shadow: var(--shadow-sm);
  transition: box-shadow var(--transition-fast);
}
.chat-bubble:hover {
  box-shadow: var(--shadow-md);
}
.chat-bubble--user {
  background: linear-gradient(135deg, var(--brand-600), var(--brand-700));
  color: white;
  border-radius: 18px 18px 4px 18px;
}
.chat-bubble--assistant {
  background: var(--bg-card);
  color: var(--text-primary);
  border: 1px solid var(--border-light);
  border-radius: 18px 18px 18px 4px;
}

.chat-timestamp {
  font-size: 11px;
  color: var(--text-muted);
}
</style>
