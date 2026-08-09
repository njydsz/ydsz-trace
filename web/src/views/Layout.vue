<template>
  <el-container class="h-screen">
    <el-aside width="220px" class="bg-brand-800 text-white">
      <div class="h-16 flex items-center px-5 text-lg font-bold tracking-wide">
        <el-icon class="mr-2"><Monitor /></el-icon>
        Ydsz Trace
      </div>
      <el-menu
        :default-active="activeMenu"
        background-color="transparent"
        text-color="#cbd5e1"
        active-text-color="#ffffff"
        router
      >
        <el-menu-item index="/dashboard">
          <el-icon><DataLine /></el-icon><span>概览</span>
        </el-menu-item>
        <el-menu-item index="/clients">
          <el-icon><Cpu /></el-icon><span>客户端管理</span>
        </el-menu-item>
        <el-menu-item index="/items">
          <el-icon><Document /></el-icon><span>日志项管理</span>
        </el-menu-item>
        <el-menu-item index="/logs">
          <el-icon><Search /></el-icon><span>日志检索</span>
        </el-menu-item>
        <el-menu-item index="/tail">
          <el-icon><VideoCamera /></el-icon><span>实时追踪</span>
        </el-menu-item>
        <el-menu-item index="/tasks">
          <el-icon><List /></el-icon><span>检索任务</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="flex items-center justify-between bg-white border-b border-slate-200">
        <div class="text-gray-700 font-medium">{{ currentTitle }}</div>
        <div class="flex items-center gap-3">
          <el-tag type="info" effect="plain" round>{{ auth.state.username }}</el-tag>
          <el-button text :icon="SwitchButton" @click="onLogout">退出</el-button>
        </div>
      </el-header>
      <el-main class="bg-slate-50">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
/**
 * 控制台框架布局：左侧 Aside 导航 + 顶部 Header（用户信息/退出）+ 右侧 Main。
 */
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Monitor, DataLine, Cpu, Document, Search, SwitchButton, VideoCamera, List } from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { logout } from '@/api/auth'
import { useAuth } from '@/store/auth'

const auth = useAuth()
const route = useRoute()
const router = useRouter()

const activeMenu = computed(() => route.path)
const currentTitle = computed(() => route.meta?.title || 'Ydsz Trace')

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
