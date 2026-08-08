<template>
  <div>
    <h2 class="text-xl font-semibold mb-4">系统概览</h2>
    <el-row :gutter="16">
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover" class="!border-brand-200">
          <div class="text-gray-500 text-sm">客户端总数</div>
          <div class="text-4xl font-bold text-brand-600 mt-2">{{ stats.clients }}</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover">
          <div class="text-gray-500 text-sm">在线客户端</div>
          <div class="text-4xl font-bold text-emerald-500 mt-2">{{ stats.online }}</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="8">
        <el-card shadow="hover">
          <div class="text-gray-500 text-sm">日志项总数</div>
          <div class="text-4xl font-bold text-brand-600 mt-2">{{ stats.items }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="mt-6">
      <template #header><span class="font-medium">快速操作</span></template>
      <div class="flex flex-wrap gap-3">
        <el-button type="primary" @click="go('/clients')">管理客户端</el-button>
        <el-button type="primary" @click="go('/items')">管理日志项</el-button>
        <el-button type="primary" @click="go('/logs')">发起日志检索</el-button>
      </div>
    </el-card>

    <el-card shadow="never" class="mt-6">
      <template #header><span class="font-medium">关于 Ydsz Trace</span></template>
      <p class="text-gray-600 leading-relaxed">
        轻量级高性能分布式日志追踪与检索系统。每台目标服务器部署 logc 客户端代理，
        通过本控制台集中管理客户端与日志项，并实时发起跨节点关键字检索，
        结果以压缩包形式回传。后端基于 Go + Gin，前端基于 Vue 3 + Vite + Element Plus + Tailwind CSS。
      </p>
    </el-card>
  </div>
</template>

/** 控制台首页：展示系统概览统计（客户端总数/在线数/日志项总数），并提供功能入口卡片。 */
<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { queryAllClients } from '@/api/client'
import { queryAllItems } from '@/api/item'

const router = useRouter()
const stats = ref({ clients: 0, online: 0, items: 0 })

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
  }
}

function go(path) {
  router.push(path)
}

onMounted(loadStats)
</script>
