// 小屏适配回归测试（V2.4-07）
//
// 背景：V2.4-07 之前，TraceWaterfallView 使用固定 min-w-[800px]，
// 导致 375px 屏出现页面级横向滚动；侧边栏在窄屏挤压主内容。
// 本测试通过源码断言防止这些回归。
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'fs'
import { join } from 'path'

// 测试在 logv 目录执行（vitest.config.js 的根目录），
// 用相对路径读取源码，避免 import.meta.url 在 Vite 转换后失效。
const layoutPath = join(process.cwd(), 'src/views/Layout.vue')
const waterfallPath = join(process.cwd(), 'src/components/TraceWaterfallView.vue')

function readSource(path) {
  return readFileSync(path, 'utf-8')
}

describe('小屏适配（V2.4-07）', () => {
  it('Layout 在窄屏使用浮层侧边栏', () => {
    const src = readSource(layoutPath)
    expect(src).toContain('isMobileView')
    expect(src).toContain('aside-mobile')
    expect(src).toContain('sidebarOpen')
  })

  it('Layout 提供窄屏断点媒体查询', () => {
    const src = readSource(layoutPath)
    expect(src).toContain('@media (max-width: 767px)')
  })

  it('Layout 提供菜单开关按钮', () => {
    const src = readSource(layoutPath)
    expect(src).toContain('Menu')
    expect(src).toContain('sidebarOpen = !sidebarOpen')
  })

  it('Layout 提供点击遮罩关闭侧边栏', () => {
    const src = readSource(layoutPath)
    expect(src).toContain('aside-mask')
    expect(src).toContain("sidebarOpen = false")
  })

  it('Layout 在窄屏禁止页面级横向溢出', () => {
    const src = readSource(layoutPath)
    expect(src).toContain('overflow-x: hidden')
    expect(src).toContain('max-width: 100vw')
  })

  it('瀑布图不再使用页面级固定宽度', () => {
    const src = readSource(waterfallPath)
    // 旧写法：min-w-[800px] 直接挂在滚动容器内，会撑宽页面
    expect(src).not.toContain('min-w-[800px]')
  })

  it('瀑布图把滚动限制在组件内部', () => {
    const src = readSource(waterfallPath)
    expect(src).toContain('overflow-x-auto')
    expect(src).toContain('w-[800px]')
  })
})
