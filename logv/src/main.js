// 应用入口：注册 Vue 3 + Pinia + Element Plus + 路由 + 全局图标
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

import App from './App.vue'
import router from './router'
import { useThemeStorage } from './store/theme'
import './style.css'

const app = createApp(App)

// 注册 Pinia（store/* 下的 defineStore 均需在此之前安装）
app.use(createPinia())

// 应用启动时初始化主题：将已保存偏好/系统偏好渲染到 DOM，并监听系统主题变更。
// 必须在 mount 之前调用，否则首屏会先用错误的主题着色再闪变。
useThemeStorage().init()

// 全局注册 Element Plus 图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(ElementPlus)
app.use(router)
app.mount('#app')
