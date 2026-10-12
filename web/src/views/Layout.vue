<template>
  <el-container class="h-screen layout-root">
    <!-- ========== 侧边栏 ========== -->
    <el-aside :width="`${sidebarWidth}px`" class="aside-custom flex flex-col">
      <!-- Logo 区域 -->
      <div class="aside-logo flex items-center gap-3">
        <div class="logo-icon flex items-center justify-center rounded-xl">
          <el-icon :size="20"><Monitor /></el-icon>
        </div>
        <span class="logo-text text-lg font-bold tracking-wide">Ydsz Trace</span>
      </div>

      <!-- 导航菜单 -->
      <nav class="aside-nav flex-1 overflow-y-auto py-2">
        <!-- 日志分析 -->
        <div class="nav-group">
          <div class="nav-group-title">日志分析</div>
          <el-menu
            :default-active="activeMenu"
            background-color="transparent"
            text-color="var(--text-aside)"
            active-text-color="var(--text-aside-active)"
            router
          >
            <el-menu-item index="/conversation">
              <div class="nav-icon-wrap"><el-icon><ChatLineRound /></el-icon></div>
              <span>对话工作台</span>
            </el-menu-item>
            <el-menu-item index="/trace">
              <div class="nav-icon-wrap"><el-icon><Share /></el-icon></div>
              <span>Trace ID 聚合</span>
            </el-menu-item>
            <el-menu-item index="/logs">
              <div class="nav-icon-wrap"><el-icon><Search /></el-icon></div>
              <span>历史日志检索</span>
            </el-menu-item>
            <el-menu-item index="/tail">
              <div class="nav-icon-wrap"><el-icon><VideoCamera /></el-icon></div>
              <span>实时日志追踪</span>
            </el-menu-item>
          </el-menu>
        </div>

        <!-- 监控运维 -->
        <div class="nav-group">
          <div class="nav-group-title">监控运维</div>
          <el-menu
            :default-active="activeMenu"
            background-color="transparent"
            text-color="var(--text-aside)"
            active-text-color="var(--text-aside-active)"
            router
          >
            <el-menu-item index="/alerts">
              <div class="nav-icon-wrap"><el-icon><Bell /></el-icon></div>
              <span>智能日志预警</span>
            </el-menu-item>
            <el-menu-item index="/tasks">
              <div class="nav-icon-wrap"><el-icon><List /></el-icon></div>
              <span>检索任务归档</span>
            </el-menu-item>
          </el-menu>
        </div>

        <!-- 系统配置 -->
        <div class="nav-group">
          <div class="nav-group-title">系统配置</div>
          <el-menu
            :default-active="activeMenu"
            background-color="transparent"
            text-color="var(--text-aside)"
            active-text-color="var(--text-aside-active)"
            router
          >
            <el-menu-item index="/clients">
              <div class="nav-icon-wrap"><el-icon><Cpu /></el-icon></div>
              <span>客户端配置</span>
            </el-menu-item>
            <el-menu-item index="/items">
              <div class="nav-icon-wrap"><el-icon><Document /></el-icon></div>
              <span>日志项配置</span>
            </el-menu-item>
            <el-menu-item index="/dashboard">
              <div class="nav-icon-wrap"><el-icon><DataLine /></el-icon></div>
              <span>平台介绍</span>
            </el-menu-item>
          </el-menu>
        </div>
      </nav>

      <!-- 底部主题切换 -->
      <div class="aside-footer flex items-center justify-center py-4">
        <el-button
          text
          :icon="themeIcon"
          :title="`主题: ${themeLabel}`"
          @click="onCycleTheme"
          class="theme-toggle"
        />
      </div>
    </el-aside>

    <!-- ========== 右侧内容区 ========== -->
    <el-container class="flex-1">
      <!-- Header -->
      <el-header class="header-custom flex items-center justify-between">
        <div class="header-left flex items-center gap-4">
          <el-breadcrumb separator="/" class="breadcrumb">
            <el-breadcrumb-item :to="breadcrumbHome.to">{{ breadcrumbHome.label }}</el-breadcrumb-item>
            <el-breadcrumb-item v-if="currentTitle !== breadcrumbHome.label">{{ currentTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right flex items-center gap-3">
          <el-button
            circle
            text
            :icon="Search"
            class="header-icon-btn"
            title="全局搜索 (Ctrl+K)"
            @click="openGlobalSearch"
          />
          <el-divider direction="vertical" class="header-divider" />
          <div class="user-info flex items-center gap-2">
            <el-avatar :size="32" class="user-avatar">
              {{ avatarText }}
            </el-avatar>
            <span class="user-name text-sm font-medium">{{ auth.state.username }}</span>
          </div>
          <el-button text :icon="SwitchButton" @click="onLogout" class="logout-btn">
            退出
          </el-button>
        </div>
      </el-header>

      <!-- Main 内容区 -->
      <el-main class="main-custom">
        <router-view v-slot="{ Component }">
          <transition name="fade-slide" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>
    </el-container>

    <!-- 全局搜索 -->
    <GlobalSearchDialog
      ref="globalSearchRef"
      v-model="globalSearchVisible"
    />
  </el-container>
</template>

<script setup>
/**
 * 控制台框架布局 — v2.0 UI 增强版。
 *
 * 左侧 Aside：分组的导航菜单 + 品牌 Logo + 主题切换。
 * 顶部 Header：面包屑导航 + 全局搜索 + 用户头像/退出。
 * 右侧 Main：路由视图 + 过渡动画。
 */
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Monitor, DataLine, Cpu, Document, Search, SwitchButton,
  VideoCamera, List, Bell, Share, Sunny, Moon, Platform, ChatLineRound,
} from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { logout } from '@/api/auth'
import { useAuth } from '@/store/auth'
import { useThemeStorage } from '@/store/theme'
import GlobalSearchDialog from '@/components/GlobalSearchDialog.vue'

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const themeStorage = useThemeStorage()
const globalSearchRef = ref(null)
const globalSearchVisible = ref(false)

const sidebarWidth = 220

const activeMenu = computed(() => route.path)
const currentTitle = computed(() => route.meta?.title || 'Ydsz Trace')
const avatarText = computed(() => (auth.state.username || 'U').slice(0, 1).toUpperCase())

const breadcrumbHome = computed(() => {
  const path = route.path
  if (path.startsWith('/conversation') || path.startsWith('/trace') || path.startsWith('/logs') || path.startsWith('/tail')) {
    return { to: { path: '/conversation' }, label: '日志分析' }
  }
  if (path.startsWith('/alerts') || path.startsWith('/tasks')) {
    return { to: { path: '/alerts' }, label: '监控运维' }
  }
  if (path.startsWith('/clients') || path.startsWith('/items') || path.startsWith('/dashboard')) {
    return { to: { path: '/clients' }, label: '系统配置' }
  }
  return { to: { path: '/conversation' }, label: '日志分析' }
})

const themeIcon = computed(() => {
  return {
    light: Sunny,
    dark: Moon,
    system: Platform,
  }[themeStorage.preference]
})

const themeLabel = computed(() => {
  return {
    light: '浅色',
    dark: '深色',
    system: '跟随系统',
  }[themeStorage.preference]
})

function onCycleTheme() {
  themeStorage.cyclePreference()
}

function openGlobalSearch() {
  globalSearchVisible.value = true
  globalSearchRef.value?.open()
}

let themeCleanup

// 全局搜索快捷键 Cmd/Ctrl+K
function handleGlobalKeydown(e) {
  const tag = e.target?.tagName?.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || e.target?.isContentEditable) return
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    openGlobalSearch()
  }
}

onMounted(() => {
  themeCleanup = themeStorage.init()
  document.addEventListener('keydown', handleGlobalKeydown)
})

onBeforeUnmount(() => {
  if (themeCleanup) themeCleanup()
  document.removeEventListener('keydown', handleGlobalKeydown)
})

async function onLogout() {
  try {
    await ElMessageBox.confirm('确认退出登录？', '提示', { type: 'warning' })
  } catch {
    return
  }
  try {
    await logout()
  } catch {
    /* 即使后端调用失败也执行本地清理 */
  }
  auth.clear()
  ElMessage.success('已退出登录')
  router.push('/login')
}
</script>

<style scoped>
.layout-root {
  --sidebar-width: 220px;
}

/* ===== 侧边栏 ===== */
.aside-custom {
  background: linear-gradient(180deg, #0f172a 0%, #0c1929 100%);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid rgba(255, 255, 255, 0.04);
}

/* Logo 区域 */
.aside-logo {
  height: var(--header-height);
  padding: 0 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}
.logo-icon {
  width: 36px;
  height: 36px;
  background: linear-gradient(135deg, var(--brand-600) 0%, var(--brand-500) 100%);
  color: white;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.35);
}
.logo-text {
  color: rgba(255, 255, 255, 0.95);
}

/* 导航区域 */
.aside-nav {
  flex: 1;
  padding: 0;
}
.nav-group {
  margin-bottom: 8px;
}
.nav-group-title {
  padding: 12px 24px 6px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--text-aside-muted);
}

/* 导航图标 */
.nav-icon-wrap {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  margin-right: 4px;
}
.el-menu-item {
  height: 40px !important;
  line-height: 40px !important;
  padding: 0 20px !important;
  margin: 2px 10px !important;
  border-radius: var(--radius-md) !important;
  font-size: 13.5px !important;
  transition: all var(--transition-fast) !important;
}
.el-menu-item:hover {
  background-color: rgba(255, 255, 255, 0.06) !important;
  color: rgba(255, 255, 255, 0.95) !important;
}
.el-menu-item.is-active {
  background: linear-gradient(90deg, rgba(37, 99, 235, 0.25) 0%, rgba(37, 99, 235, 0.1) 100%) !important;
  color: #ffffff !important;
  font-weight: 600 !important;
  box-shadow: none !important;
  position: relative;
}
.el-menu-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  bottom: 8px;
  width: 3px;
  background: var(--brand-500);
  border-radius: 0 3px 3px 0;
}

/* 底部 */
.aside-footer {
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}
.theme-toggle {
  color: var(--text-aside) !important;
  font-size: 18px;
  padding: 8px 16px !important;
  border-radius: var(--radius-md) !important;
  transition: all var(--transition-fast);
}
.theme-toggle:hover {
  color: white !important;
  background: rgba(255, 255, 255, 0.08);
}

/* ===== Header ===== */
.header-custom {
  background-color: var(--bg-header);
  border-bottom: 1px solid var(--border-header);
  height: var(--header-height) !important;
  padding: 0 24px !important;
}
.breadcrumb :deep(.el-breadcrumb__inner) {
  color: var(--text-secondary);
  font-size: 13px;
}
.breadcrumb :deep(.el-breadcrumb__inner a) {
  color: var(--text-muted);
  font-weight: 500;
}
.breadcrumb :deep(.el-breadcrumb__inner a:hover) {
  color: var(--brand-600);
}
.breadcrumb :deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: var(--text-primary);
  font-weight: 600;
}

.header-icon-btn {
  font-size: 16px;
  color: var(--text-secondary) !important;
}
.header-icon-btn:hover {
  color: var(--brand-600) !important;
  background: var(--brand-50) !important;
}
.header-divider {
  height: 20px;
  margin: 0 4px !important;
  border-color: var(--border-light) !important;
}

/* 用户信息 */
.user-info {
  cursor: default;
  padding: 2px 8px 2px 4px;
  border-radius: var(--radius-md);
  transition: background var(--transition-fast);
}
.user-info:hover {
  background: var(--bg-card-muted);
}
.user-avatar {
  background: linear-gradient(135deg, var(--brand-500) 0%, var(--brand-700) 100%);
  color: white;
  font-weight: 600;
  font-size: 14px;
}
.user-name {
  color: var(--text-primary);
}
.logout-btn {
  color: var(--text-secondary) !important;
  font-size: 13px !important;
}
.logout-btn:hover {
  color: var(--danger) !important;
}

/* ===== Main ===== */
.main-custom {
  background-color: var(--bg-main);
  padding: 20px 24px !important;
  overflow-y: auto;
}

/* 页面过渡动画 */
.fade-slide-enter-active,
.fade-slide-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}
.fade-slide-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.fade-slide-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
