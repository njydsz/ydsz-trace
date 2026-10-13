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
              <span>日志检索助手</span>
            </el-menu-item>
            <el-menu-item index="/trace">
              <div class="nav-icon-wrap"><el-icon><Share /></el-icon></div>
              <span>日志链路追踪</span>
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
              <span>日志节点配置</span>
            </el-menu-item>
            <el-menu-item index="/items">
              <div class="nav-icon-wrap"><el-icon><Document /></el-icon></div>
              <span>采集路径配置</span>
            </el-menu-item>
            <el-menu-item index="/model-capability">
              <div class="nav-icon-wrap"><el-icon><MagicStick /></el-icon></div>
              <span>模型能力配置</span>
            </el-menu-item>
          </el-menu>
        </div>

        <!-- 关于我们 -->
        <div class="nav-group">
          <div class="nav-group-title">关于我们</div>
          <el-menu
            :default-active="activeMenu"
            background-color="transparent"
            text-color="var(--text-aside)"
            active-text-color="var(--text-aside-active)"
            router
          >
            <el-menu-item index="/dashboard">
              <div class="nav-icon-wrap"><el-icon><DataLine /></el-icon></div>
              <span>云顶日志平台</span>
            </el-menu-item>
            <el-menu-item @click="onOpenExternal('https://njydsz.com')">
              <div class="nav-icon-wrap"><el-icon><InfoFilled /></el-icon></div>
              <span>南京云顶数字</span>
            </el-menu-item>
          </el-menu>
        </div>
      </nav>

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
          <el-dropdown trigger="click" @command="onUserCommand" class="user-dropdown">
            <div class="user-info flex items-center gap-2">
              <el-avatar :size="32" class="user-avatar">
                {{ avatarText }}
              </el-avatar>
              <span class="user-name text-sm font-medium">{{ auth.state.username }}</span>
              <el-icon :size="12" class="dropdown-arrow"><ArrowDown /></el-icon>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="changePassword" :icon="EditPen">
                  修改密码
                </el-dropdown-item>
                <el-dropdown-item command="logout" :icon="SwitchButton" divided>
                  退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
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

    <!-- 修改密码弹窗 -->
    <el-dialog
      v-model="passwordDialogVisible"
      title="修改密码"
      width="420px"
      :close-on-click-modal="false"
      class="password-dialog"
    >
      <ElForm
        ref="passwordFormRef"
        :model="passwordForm"
        :rules="passwordRules"
        label-position="top"
      >
        <ElFormItem label="旧密码" prop="oldPassword">
          <ElInput
            v-model="passwordForm.oldPassword"
            type="password"
            placeholder="请输入当前密码"
            show-password
            autocomplete="current-password"
          />
        </ElFormItem>
        <ElFormItem label="新密码" prop="newPassword">
          <ElInput
            v-model="passwordForm.newPassword"
            type="password"
            placeholder="至少 6 位"
            show-password
            autocomplete="new-password"
          />
        </ElFormItem>
        <ElFormItem label="确认新密码" prop="confirmPassword">
          <ElInput
            v-model="passwordForm.confirmPassword"
            type="password"
            placeholder="请再次输入新密码"
            show-password
            autocomplete="new-password"
          />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <el-button @click="passwordDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="passwordLoading" @click="submitChangePassword">确认修改</el-button>
      </template>
    </el-dialog>
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
  VideoCamera, List, Bell, Share, ChatLineRound, ArrowDown, EditPen, MagicStick,
  InfoFilled,
} from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage, ElForm, ElFormItem, ElInput } from 'element-plus'
import { logout, changePassword } from '@/api/auth'
import { useAuth } from '@/store/auth'
import GlobalSearchDialog from '@/components/GlobalSearchDialog.vue'

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const globalSearchRef = ref(null)
const globalSearchVisible = ref(false)
const passwordDialogVisible = ref(false)
const passwordFormRef = ref(null)
const passwordLoading = ref(false)
const passwordForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '' })
const passwordRules = {
  oldPassword: [{ required: true, message: '请输入旧密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码至少 6 位', trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (value !== passwordForm.value.newPassword) {
          callback(new Error('两次输入的密码不一致'))
        } else {
          callback()
        }
      },
      trigger: 'blur',
    },
  ],
}

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
  if (path.startsWith('/dashboard')) {
    return { to: { path: '/dashboard' }, label: '关于我们' }
  }
  if (path.startsWith('/clients') || path.startsWith('/items') || path.startsWith('/model-capability')) {
    return { to: { path: '/clients' }, label: '系统配置' }
  }
  return { to: { path: '/conversation' }, label: '日志分析' }
})

function openGlobalSearch() {
  globalSearchVisible.value = true
  globalSearchRef.value?.open()
}

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
  document.addEventListener('keydown', handleGlobalKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleGlobalKeydown)
})

function onUserCommand(cmd) {
  if (cmd === 'changePassword') {
    openPasswordDialog()
  } else if (cmd === 'logout') {
    onLogout()
  }
}

function openPasswordDialog() {
  passwordForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
  passwordDialogVisible.value = true
}

async function submitChangePassword() {
  if (!passwordFormRef.value) return
  try {
    await passwordFormRef.value.validate()
  } catch {
    return
  }
  passwordLoading.value = true
  try {
    await changePassword(passwordForm.value.oldPassword, passwordForm.value.newPassword)
    ElMessage.success('密码修改成功，请重新登录')
    passwordDialogVisible.value = false
    // 密码修改成功后清除本地会话并跳转登录
    auth.clear()
    router.push('/login')
  } catch (err) {
    const msg = err?.response?.data?.msg || err?.message || '密码修改失败'
    ElMessage.error(msg)
  } finally {
    passwordLoading.value = false
  }
}

function onOpenExternal(url) {
  if (url) {
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}

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
  background: var(--bg-aside);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid var(--border-light);
}

/* Logo 区域 */
.aside-logo {
  height: var(--header-height);
  padding: 0 16px;
  border-bottom: 1px solid var(--border-light);
  flex-shrink: 0;
}
.logo-icon {
  width: 36px;
  height: 36px;
  background: linear-gradient(135deg, var(--brand-600) 0%, var(--brand-500) 100%);
  color: white;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.25);
}
.logo-text {
  color: var(--text-primary);
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
  padding: 14px 24px 6px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: none;
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
  background-color: var(--bg-aside-hover) !important;
  color: var(--text-primary) !important;
}
.el-menu-item.is-active {
  background: var(--bg-aside-active) !important;
  color: var(--brand-700) !important;
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
  background: var(--brand-600);
  border-radius: 0 3px 3px 0;
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

/* 用户信息 / 下拉 */
.user-dropdown {
  outline: none;
}
.user-info {
  cursor: pointer;
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
.dropdown-arrow {
  color: var(--text-muted);
  transition: transform var(--transition-fast);
}
:deep(.el-dropdown-menu__item:hover) {
  color: var(--brand-600) !important;
  background: var(--brand-50) !important;
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
