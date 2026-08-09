import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// Vitest 配置 — 共用 Vite 的 alias 与 Vue 插件，jsdom 环境模拟浏览器全局。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['**/*.{spec,test}.{js,jsx,mjs,cjs}'],
    exclude: ['node_modules', 'dist', 'coverage', 'e2e'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      include: ['src/**/*.{js,vue}'],
      exclude: ['src/main.js', 'src/App.vue', '**/node_modules/**'],
    },
  },
})
