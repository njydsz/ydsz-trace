import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// 开发模式下，前端跑在 5173，通过代理把 API 请求转发到 logs 服务端 (2021)。
// 生产模式下由 Go 服务端直接托管 web/dist，前后端同域，无需代理。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/admin': { target: 'http://localhost:2021', changeOrigin: true },
      '/client': { target: 'http://localhost:2021', changeOrigin: true },
      '/item': { target: 'http://localhost:2021', changeOrigin: true },
      '/logs': { target: 'http://localhost:2021', changeOrigin: true },
      '/health': { target: 'http://localhost:2021', changeOrigin: true },
      '/ready': { target: 'http://localhost:2021', changeOrigin: true },
    },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1500,
  },
})
