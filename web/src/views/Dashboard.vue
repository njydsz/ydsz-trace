<template>
  <div class="dashboard">
    <!-- 欢迎横幅 -->
    <div class="welcome-banner rounded-2xl p-6 mb-6">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="welcome-title">欢迎使用 Ydsz Trace</h1>
          <p class="welcome-desc mt-1">AI 驱动的分布式日志排障定位平台 — 让日志分析更智能、更高效</p>
        </div>
        <div class="welcome-icon hidden sm:flex">
          <el-icon :size="48"><DataAnalysis /></el-icon>
        </div>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
      <!-- 客户端总数 -->
      <div class="stat-card stat-card--primary rounded-xl p-5">
        <div class="flex items-start justify-between">
          <div>
            <div class="stat-label">客户端总数</div>
            <div class="stat-value mt-2">
              <el-skeleton :rows="0" animated v-if="loading" style="width: 80px; height: 40px;" />
              <span v-else>{{ stats.clients }}</span>
            </div>
          </div>
          <div class="stat-icon stat-icon--primary flex items-center justify-center rounded-xl">
            <el-icon :size="22"><Cpu /></el-icon>
          </div>
        </div>
        <div class="stat-footer mt-3">
          <span class="stat-footer-text">已注册节点</span>
        </div>
      </div>

      <!-- 在线客户端 -->
      <div class="stat-card stat-card--success rounded-xl p-5">
        <div class="flex items-start justify-between">
          <div>
            <div class="stat-label">在线客户端</div>
            <div class="stat-value mt-2">
              <el-skeleton :rows="0" animated v-if="loading" style="width: 80px; height: 40px;" />
              <span v-else>{{ stats.online }}</span>
            </div>
          </div>
          <div class="stat-icon stat-icon--success flex items-center justify-center rounded-xl">
            <el-icon :size="22"><Connection /></el-icon>
          </div>
        </div>
        <div class="stat-footer mt-3">
          <span class="stat-badge" :class="stats.online > 0 ? 'stat-badge--online' : 'stat-badge--offline'">
            {{ stats.online > 0 ? '运行中' : '无连接' }}
          </span>
        </div>
      </div>

      <!-- 日志项总数 -->
      <div class="stat-card stat-card--info rounded-xl p-5">
        <div class="flex items-start justify-between">
          <div>
            <div class="stat-label">日志项总数</div>
            <div class="stat-value mt-2">
              <el-skeleton :rows="0" animated v-if="loading" style="width: 80px; height: 40px;" />
              <span v-else>{{ stats.items }}</span>
            </div>
          </div>
          <div class="stat-icon stat-icon--info flex items-center justify-center rounded-xl">
            <el-icon :size="22"><Document /></el-icon>
          </div>
        </div>
        <div class="stat-footer mt-3">
          <span class="stat-footer-text">可检索日志源</span>
        </div>
      </div>
    </div>

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
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" @click="go('/conversation')">
            <div class="quick-action-icon quick-action-icon--primary flex items-center justify-center rounded-xl">
              <el-icon :size="20"><ChatLineRound /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">对话工作台</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" @click="go('/clients')">
            <div class="quick-action-icon quick-action-icon--success flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Cpu /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">客户端配置</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" @click="go('/items')">
            <div class="quick-action-icon quick-action-icon--info flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Document /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">日志项配置</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" @click="go('/logs')">
            <div class="quick-action-icon quick-action-icon--warning flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Search /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">历史检索</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" @click="go('/trace')">
            <div class="quick-action-icon quick-action-icon--danger flex items-center justify-center rounded-xl">
              <el-icon :size="20"><Share /></el-icon>
            </div>
            <span class="quick-action-label text-xs font-medium">Trace 追踪</span>
          </button>
          <button class="quick-action flex flex-col items-center gap-2 rounded-xl p-4" @click="go('/alerts')">
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
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup>
/**
 * 控制台首页 v2.0 — 展示统计卡片（客户端/在线数/日志项总数）、快捷操作面板和平台信息。
 * 增强版：添加品牌色卡片、彩色图标背景、快捷操作按钮、技术栈标签。
 */
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  Cpu, Document, Connection, Search, Share, Bell,
  ChatLineRound, Lightning, InfoFilled, DataAnalysis,
} from '@element-plus/icons-vue'
import { queryAllClients } from '@/api/client'
import { queryAllItems } from '@/api/item'

const router = useRouter()
const stats = ref({ clients: 0, online: 0, items: 0 })
const loading = ref(true)

async function loadStats() {
  try {
    const c = await queryAllClients()
    const arr = Array.isArray(c.data) ? c.data : c.data?.data || []
    stats.value.clients = arr.length
    stats.value.online = arr.filter((x) => x.online === '1').length
  } catch {
    /* 忽略统计错误 */
  }
  try {
    const i = await queryAllItems()
    const arr = Array.isArray(i.data) ? i.data : i.data?.data || []
    stats.value.items = arr.length
  } catch {
    /* 忽略统计错误 */
  } finally {
    loading.value = false
  }
}

function go(path) {
  router.push(path)
}

onMounted(loadStats)
</script>

<style scoped>
/* ===== 欢迎横幅 ===== */
.welcome-banner {
  background: linear-gradient(135deg, var(--brand-700) 0%, var(--brand-600) 50%, var(--brand-500) 100%);
  color: white;
  position: relative;
  overflow: hidden;
}
.welcome-banner::before {
  content: '';
  position: absolute;
  inset: 0;
  background: radial-gradient(circle at 80% 50%, rgba(255, 255, 255, 0.08) 0%, transparent 60%);
  pointer-events: none;
}
.welcome-title {
  font-size: 20px;
  font-weight: 700;
  margin: 0;
}
.welcome-desc {
  font-size: 13.5px;
  opacity: 0.85;
  margin: 0;
}
.welcome-icon {
  opacity: 0.5;
  background: rgba(255, 255, 255, 0.1);
  width: 72px;
  height: 72px;
  border-radius: 20px;
  align-items: center;
  justify-content: center;
}

/* ===== 统计卡片 ===== */
.stat-card {
  background: var(--bg-card);
  border: 1px solid var(--border-card);
  box-shadow: var(--shadow-card);
  transition: box-shadow var(--transition-fast), transform var(--transition-fast);
}
.stat-card:hover {
  box-shadow: var(--shadow-card-hover);
  transform: translateY(-2px);
}
.stat-label {
  font-size: 13px;
  color: var(--text-secondary);
  font-weight: 500;
}
.stat-value {
  font-size: 36px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1;
}

/* 图标背景 */
.stat-icon {
  width: 48px;
  height: 48px;
  color: white;
}
.stat-icon--primary {
  background: linear-gradient(135deg, var(--brand-500), var(--brand-700));
  box-shadow: 0 4px 10px rgba(37, 99, 235, 0.25);
}
.stat-icon--success {
  background: linear-gradient(135deg, #10b981, #059669);
  box-shadow: 0 4px 10px rgba(16, 185, 129, 0.25);
}
.stat-icon--info {
  background: linear-gradient(135deg, #6366f1, #4f46e5);
  box-shadow: 0 4px 10px rgba(99, 102, 241, 0.25);
}

/* 卡片底部 */
.stat-footer {
  display: flex;
  align-items: center;
  gap: 8px;
}
.stat-footer-text {
  font-size: 12px;
  color: var(--text-muted);
}
.stat-badge {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: var(--radius-full);
  line-height: 20px;
}
.stat-badge--online {
  background: var(--success-light);
  color: #059669;
}
.stat-badge--offline {
  background: var(--danger-light);
  color: #dc2626;
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
</style>
