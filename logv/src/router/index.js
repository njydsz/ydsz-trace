// 前端路由配置
//
// 包含两段：公开路由（/login）+ 需登录路由（/ 下的 dashboard/clients/items/logs）。
// 通过 beforeEach 守卫实现未登录跳转与已登录访问 /login 的自动回跳。
import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from '@/store/auth'

import Login from '@/views/Login.vue'
import Layout from '@/views/Layout.vue'
import Dashboard from '@/views/Dashboard.vue'
import Clients from '@/views/Clients.vue'
import Items from '@/views/Items.vue'
import LogSearch from '@/views/LogSearch.vue'
import LogTail from '@/views/LogTail.vue'
import Alerts from '@/views/Alerts.vue'
import TaskList from '@/views/TaskList.vue'
import TraceView from '@/views/TraceView.vue'
import ConversationWorkbench from '@/views/ConversationWorkbench.vue'
import ModelCapability from '@/views/ModelCapability.vue'

const routes = [
  // 公开路由：登录页。
  { path: '/login', name: 'login', component: Login, meta: { public: true } },
  {
    path: '/',
    component: Layout,
    redirect: '/conversation',
    children: [
      // 默认首页：对话式工作台（v2.0+）。
      { path: 'conversation', name: 'conversation', component: ConversationWorkbench, meta: { title: '日志检索助手' } },
      // 控制台子路由（均需登录）。
      { path: 'trace', name: 'trace', component: TraceView, meta: { title: '日志链路追踪' } },
      { path: 'logs', name: 'logs', component: LogSearch, meta: { title: '历史日志检索' } },
      { path: 'tail', name: 'tail', component: LogTail, meta: { title: '实时日志追踪' } },
      { path: 'alerts', name: 'alerts', component: Alerts, meta: { title: '智能日志预警' } },
      { path: 'tasks', name: 'tasks', component: TaskList, meta: { title: '检索任务归档' } },
      { path: 'clients', name: 'clients', component: Clients, meta: { title: '日志节点配置' } },
      { path: 'items', name: 'items', component: Items, meta: { title: '采集路径配置' } },
      { path: 'model-capability', name: 'model-capability', component: ModelCapability, meta: { title: '模型能力配置' } },
      { path: 'dashboard', name: 'dashboard', component: Dashboard, meta: { title: '云顶日志平台' } },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// roleRouteRules 角色 → 允许访问的路由路径前缀。
// viewer: 只读（可查看日志、检索、对话等，但不可配置资源）
// operator: 运维（可管理客户端、日志项、发起查询，但不可配置模型能力）
// admin: 全部权限
const roleRouteRules = {
  viewer: [
    '/conversation', '/trace', '/logs', '/tail', '/alerts', '/tasks', '/dashboard',
  ],
  operator: [
    '/conversation', '/trace', '/logs', '/tail', '/alerts', '/tasks',
    '/clients', '/items', '/dashboard',
  ],
  admin: [
    '/conversation', '/trace', '/logs', '/tail', '/alerts', '/tasks',
    '/clients', '/items', '/model-capability', '/dashboard',
  ],
}

// hasRouteAccess 检查指定角色是否有权访问目标路径。
function hasRouteAccess(role, path) {
  const rules = roleRouteRules[role] || roleRouteRules.viewer
  return rules.some(prefix => path === prefix || path.startsWith(prefix + '/'))
}

router.beforeEach((to) => {
  const { loggedIn, role } = useAuth()
  // 未登录访问非公开路由：跳登录页，保留 redirect query 用于登录后回跳。
  if (!to.meta.public && !loggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && loggedIn) {
    return { name: 'dashboard' }
  }
  // 角色权限校验：已登录用户访问受限页面时重定向到首页
  if (loggedIn && !to.meta.public && to.path !== '/') {
    if (!hasRouteAccess(role, to.path)) {
      return { name: 'conversation' }
    }
  }
  return true
})

export default router
