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

const routes = [
  // 公开路由：登录页。
  { path: '/login', name: 'login', component: Login, meta: { public: true } },
  {
    path: '/',
    component: Layout,
    redirect: '/dashboard',
    children: [
      // 控制台子路由（均需登录）。
      { path: 'dashboard', name: 'dashboard', component: Dashboard, meta: { title: '概览' } },
      { path: 'clients', name: 'clients', component: Clients, meta: { title: '客户端管理' } },
      { path: 'items', name: 'items', component: Items, meta: { title: '日志项管理' } },
      { path: 'logs', name: 'logs', component: LogSearch, meta: { title: '日志检索' } },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const { loggedIn } = useAuth()
  // 未登录访问非公开路由：跳登录页，保留 redirect query 用于登录后回跳。
  if (!to.meta.public && !loggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && loggedIn) {
    return { name: 'dashboard' }
  }
  return true
})

export default router
