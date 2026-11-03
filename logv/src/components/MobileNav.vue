<template>
  <nav
    class="fixed bottom-0 left-0 right-0 z-50 bg-white border-t border-gray-200 md:hidden"
    :style="{ paddingBottom: 'max(0.5rem, env(safe-area-inset-bottom))' }"
  >
    <ul class="flex justify-around items-center h-14">
      <li v-for="tab in tabs" :key="tab.path" class="flex-1">
        <router-link
          :to="tab.path"
          class="flex flex-col items-center justify-center gap-0.5 h-full text-gray-500 text-xs hover:text-brand-600 transition-colors"
          active-class="!text-brand-600 font-medium"
        >
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <component :is="tab.icon" />
          </svg>
          <span>{{ tab.label }}</span>
        </router-link>
      </li>
    </ul>
  </nav>
</template>

<script setup>
// 移动端底部导航。
//
// V2.1-07 修复：原先指向 /search /stats /history /settings 等不存在的路由，
// 点击后全部落到 catch-all → /dashboard，属于死链。现改为真实存在的路由。
//
// 路径必须与 router/index.js 中的子路由一致：
//   /conversation 对话工作台、/logs 历史检索、/tail 实时追踪、
//   /alerts 预警、/dashboard 平台总览
import { h } from 'vue'

function icon(paths) {
  return () => h('svg', {
    width: '22',
    height: '22',
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    'stroke-width': '2',
    'stroke-linecap': 'round',
    'stroke-linejoin': 'round',
  }, paths.map((d) => h('path', { d })))
}

const tabs = [
  {
    path: '/conversation',
    label: '对话',
    icon: icon(['M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z']),
  },
  {
    path: '/logs',
    label: '检索',
    icon: icon(['M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z', 'M21 21l-4.35-4.35']),
  },
  {
    path: '/tail',
    label: '实时',
    icon: icon(['M13 2L3 14h9l-1 8 10-12h-9l1-8z']),
  },
  {
    path: '/alerts',
    label: '预警',
    icon: icon(['M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9', 'M13.73 21a2 2 0 0 1-3.46 0']),
  },
  {
    path: '/dashboard',
    label: '总览',
    icon: icon(['M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z', 'M9 22V12h6v10']),
  },
]
</script>
