<template>
  <div class="dashboard">
    <!-- 分时段欢迎问候条（含平台标语） -->
    <div class="greeting-banner flex items-center gap-3 rounded-xl px-4 py-3 mb-4" data-testid="dashboard-greeting">
      <el-icon :size="20" class="greeting-icon">
        <component :is="greetingIcon" />
      </el-icon>
      <div class="greeting-main min-w-0">
        <span class="greeting-text" data-testid="dashboard-greeting-text">{{ greeting }}！欢迎回来，祝您工作顺利</span>
        <span class="greeting-slogan" data-testid="dashboard-welcome-desc">AI 驱动的分布式日志排障定位平台 — 让日志分析更智能、更高效</span>
      </div>
      <span class="greeting-brand" data-testid="dashboard-welcome-title">Ydsz Trace</span>
      <span class="greeting-time ml-auto">{{ new Date().toLocaleString('zh-CN', { hour: '2-digit', minute: '2-digit', month: 'long', day: 'numeric', weekday: 'long' }) }}</span>
    </div>

    <!-- AI 四层智能引擎状态 -->
    <div class="mb-6">
      <AIStatusPanel />
    </div>

    <!-- 洞察看板（模板趋势 / 异常事件时间线 / 服务健康度总览） -->
    <div v-if="!insightLoaded" class="mb-6" data-testid="dashboard-insight-skeleton">
      <SkeletonLoader type="card" :count="2" />
    </div>
    <DashboardInsight v-show="insightLoaded" data-testid="dashboard-insight" @loaded="onInsightLoaded" />

    <!-- 快捷操作 + 关于 -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <!-- 快捷操作 -->
      <el-card shadow="never" class="!rounded-xl">
        <template #header>
          <div class="flex items-center gap-2">
            <el-icon class="text-brand-600"><Lightning /></el-icon>
            <span class="font-semibold text-sm">快捷操作</span>
          </div>
        </template>
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3" data-testid="dashboard-quick-actions">
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" data-testid="quick-action-conversation" @click="go('/conversation')">
            <div class="quick-action-icon quick-action-icon--primary flex items-center justify-center rounded-xl">
              <el-icon :size="20"><ChatLineRound /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">日志检索助手</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" data-testid="quick-action-clients" @click="go('/clients')">
            <div class="quick-action-icon quick-action-icon--success flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Cpu /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">日志节点配置</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" data-testid="quick-action-items" @click="go('/items')">
            <div class="quick-action-icon quick-action-icon--info flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Document /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">采集路径配置</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" data-testid="quick-action-logs" @click="go('/logs')">
            <div class="quick-action-icon quick-action-icon--warning flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Search /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">历史检索</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" data-testid="quick-action-trace" @click="go('/trace')">
            <div class="quick-action-icon quick-action-icon--danger flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Share /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">日志链路追踪</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" data-testid="quick-action-alerts" @click="go('/alerts')">
            <div class="quick-action-icon quick-action-icon--purple flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Bell /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">日志预警</span>
          </button>
        </div>
      </el-card>

      <!-- 关于 -->
      <el-card shadow="never" class="!rounded-xl">
        <template #header>
          <div class="flex items-center gap-2">
            <el-icon class="text-brand-600"><InfoFilled /></el-icon>
            <span class="font-semibold text-sm">关于平台</span>
          </div>
        </template>
        <div class="about-content">
          <p class="about-text leading-relaxed">
            <strong class="text-brand-700">Ydsz Trace</strong> 是轻量级高性能分布式日志追踪与检索系统。
            每台目标服务器部署 logc 客户端代理，通过本控制台集中管理客户端与日志项，
            支持实时跨节点关键字检索与 AI 驱动的自然语言排障。
          </p>
          <div class="tech-stack mt-4 flex flex-wrap gap-2">
            <span class="tech-tag">Go 1.26</span>
            <span class="tech-tag">Vue 3</span>
            <span class="tech-tag">Gin</span>
            <span class="tech-tag">SQLite</span>
            <span class="tech-tag">Vite 6</span>
            <span class="tech-tag">Tailwind CSS</span>
          </div>
          <div class="repo-links mt-4">
            <a href="https://gitee.com/njydsz/ydsz-trace" target="_blank" rel="noopener noreferrer" class="repo-link">
              <el-icon :size="14"><Position /></el-icon>
              <span>Gitee 仓库</span>
            </a>
            <a href="https://github.com/njydsz/ydsz-trace" target="_blank" rel="noopener noreferrer" class="repo-link">
              <el-icon :size="14"><Position /></el-icon>
              <span>GitHub 仓库</span>
            </a>
          </div>
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup>
/**
 * 控制台首页 v2.0 — 洞察看板（模板趋势 + 异常事件时间线 + 服务健康度总览）
 * + 快捷操作面板 + 平台关于信息。
 */
import { useRouter } from 'vue-router'
import {
  Search, Share, Bell,
  ChatLineRound, Lightning, InfoFilled, Position,
} from '@element-plus/icons-vue'
import DashboardInsight from '@/components/DashboardInsight.vue'
import SkeletonLoader from '@/components/SkeletonLoader.vue'
import AIStatusPanel from '@/components/AIStatusPanel.vue'
import { ref, computed } from 'vue'
import { Sunrise, PartlyCloudy, Sunset, Moon } from '@element-plus/icons-vue'

const router = useRouter()

function go(path) {
  router.push(path)
}

// 分时段问候
const hour = computed(() => new Date().getHours())
const greeting = computed(() => {
  if (hour.value >= 5 && hour.value < 12) return '早上好'
  if (hour.value >= 12 && hour.value < 18) return '下午好'
  if (hour.value >= 18 && hour.value < 22) return '晚上好'
  return '夜深了'
})
const greetingIcon = computed(() => {
  if (hour.value >= 5 && hour.value < 12) return Sunrise
  if (hour.value >= 12 && hour.value < 18) return PartlyCloudy
  if (hour.value >= 18 && hour.value < 22) return Sunset
  return Moon
})

// DashboardInsight 加载状态
const insightLoaded = ref(false)
function onInsightLoaded() {
  insightLoaded.value = true
}
</script>

<style scoped>
/* ===== 分时段问候横幅 ===== */
.greeting-banner {
  background: linear-gradient(135deg, #eff6ff 0%, #e0f2fe 100%);
  border: 1px solid #bfdbfe;
}
.greeting-banner .greeting-icon {
  color: #2563eb;
  flex-shrink: 0;
}
.greeting-main {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}
.greeting-text {
  font-size: 14px;
  font-weight: 500;
  color: #1e40af;
  line-height: 1.2;
}
.greeting-slogan {
  font-size: 12px;
  color: #3b82f6;
  line-height: 1.3;
}
.greeting-brand {
  font-size: 12px;
  font-weight: 600;
  color: #1e40af;
  background: rgba(255, 255, 255, 0.65);
  border: 1px solid #bfdbfe;
  border-radius: 8px;
  padding: 3px 10px;
  white-space: nowrap;
}
.greeting-time {
  font-size: 12px;
  color: #6b7280;
  white-space: nowrap;
}

/* ===== 快捷操作 ===== */
.quick-action {
  background: var(--bg-card-muted);
  border: 1px solid transparent;
  cursor: pointer;
  transition: all var(--transition-fast);
  outline: none;
}
.quick-action:hover {
  background: white;
  border-color: var(--brand-200);
  box-shadow: var(--shadow-md);
  transform: translateY(-2px);
}
.quick-action:active {
  transform: translateY(0);
}
.quick-action-icon {
  width: 44px;
  height: 44px;
  color: white;
}
.quick-action-icon--primary {
  background: linear-gradient(135deg, var(--brand-500), var(--brand-700));
}
.quick-action-icon--success {
  background: linear-gradient(135deg, #10b981, #059669);
}
.quick-action-icon--info {
  background: linear-gradient(135deg, #6366f1, #4f46e5);
}
.quick-action-icon--warning {
  background: linear-gradient(135deg, #f59e0b, #d97706);
}
.quick-action-icon--danger {
  background: linear-gradient(135deg, #ef4444, #dc2626);
}
.quick-action-icon--purple {
  background: linear-gradient(135deg, #8b5cf6, #7c3aed);
}
.quick-action-label {
  color: var(--text-secondary);
}

/* ===== 关于 ===== */
.about-text {
  font-size: 13.5px;
  color: var(--text-secondary);
  margin: 0;
}
.tech-tag {
  display: inline-block;
  padding: 3px 10px;
  font-size: 12px;
  font-weight: 500;
  color: var(--brand-700);
  background: var(--brand-50);
  border-radius: var(--radius-sm);
  border: 1px solid var(--brand-100);
}

/* ===== 仓库链接 ===== */
.repo-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.repo-link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 12px;
  font-size: 12.5px;
  font-weight: 500;
  color: var(--brand-700);
  background: var(--brand-50);
  border-radius: var(--radius-sm);
  border: 1px solid var(--brand-100);
  text-decoration: none;
  transition: all var(--transition-fast);
}
.repo-link:hover {
  background: var(--brand-100);
  border-color: var(--brand-200);
  color: var(--brand-800);
}
.repo-link .el-icon {
  flex-shrink: 0;
}
</style>
