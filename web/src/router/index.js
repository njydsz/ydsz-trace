import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from '@/store/auth'

import Login from '@/views/Login.vue'
import Layout from '@/views/Layout.vue'
import Dashboard from '@/views/Dashboard.vue'
import Clients from '@/views/Clients.vue'
import Items from '@/views/Items.vue'
import LogSearch from '@/views/LogSearch.vue'

const routes = [
  { path: '/login', name: 'login', component: Login, meta: { public: true } },
  {
    path: '/',
    component: Layout,
    redirect: '/dashboard',
    children: [
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
  if (!to.meta.public && !loggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && loggedIn) {
    return { name: 'dashboard' }
  }
  return true
})

export default router
