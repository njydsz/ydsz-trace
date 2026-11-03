// MobileNav 单元测试（V2.1-07 补齐）
//
// 覆盖：tab 路径必须与真实路由一致（防死链回归）。
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import MobileNav from './MobileNav.vue'

// 与 router/index.js 保持一致的真实路由表
const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', redirect: '/conversation' },
    { path: '/conversation', component: { template: '<div />' } },
    { path: '/logs', component: { template: '<div />' } },
    { path: '/tail', component: { template: '<div />' } },
    { path: '/alerts', component: { template: '<div />' } },
    { path: '/dashboard', component: { template: '<div />' } },
  ],
})

describe('MobileNav', () => {
  it('渲染 5 个 tab', () => {
    const wrapper = mount(MobileNav, { global: { plugins: [router] } })
    expect(wrapper.findAll('li')).toHaveLength(5)
  })

  it('所有 tab 路径都能在路由表中解析（无死链）', async () => {
    await router.push('/')
    await router.isReady()

    const wrapper = mount(MobileNav, { global: { plugins: [router] } })
    const links = wrapper.findAll('a')
    const paths = links.map((a) => new URL(a.attributes('href'), 'http://localhost').pathname)

    for (const path of paths) {
      const resolved = router.resolve(path)
      // 未匹配的路由会落到 catch-all / redirect，此处断言存在具名命中的组件
      expect(resolved.matched.length).toBeGreaterThan(0)
      expect(resolved.matched[resolved.matched.length - 1].components?.default).toBeTruthy()
    }
  })

  it('包含检索 / 实时 / 预警 / 总览 四个核心入口', () => {
    const wrapper = mount(MobileNav, { global: { plugins: [router] } })
    const labels = wrapper.findAll('span').map((s) => s.text())
    expect(labels).toContain('检索')
    expect(labels).toContain('实时')
    expect(labels).toContain('预警')
    expect(labels).toContain('总览')
  })
})
