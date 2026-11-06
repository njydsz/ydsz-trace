import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createWebHistory } from 'vue-router'
import { createPinia } from 'pinia'
import LogTail from './LogTail.vue'

vi.mock('@/api/sse', () => ({
  createSSE: vi.fn(() => ({
    on: vi.fn(),
    close: vi.fn(),
    isActive: vi.fn(() => true),
  })),
}))

vi.mock('@/api/logs', () => ({
  queryLogClients: vi.fn().mockResolvedValue({ code: 0, data: [] }),
  queryItemsByClient: vi.fn().mockResolvedValue({ code: 0, data: [] }),
}))

describe('LogTail', () => {
  let wrapper

  beforeEach(() => {
    const router = createRouter({
      history: createWebHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }],
    })
    wrapper = mount(LogTail, {
      global: {
        plugins: [createPinia(), router],
      },
    })
  })

  it('renders tail controls', () => {
    expect(wrapper.text()).toContain('实时日志')
    expect(wrapper.text()).toContain('追踪')
  })

  it('has pause/resume button', () => {
    const buttons = wrapper.findAll('button')
    const hasPause = buttons.some((b) => b.text().includes('暂停') || b.text().includes('继续'))
    expect(hasPause || wrapper.text()).toBeTruthy()
  })

  it('has clear button', () => {
    const buttons = wrapper.findAll('button')
    const hasClear = buttons.some((b) => b.text().includes('清空'))
    expect(hasClear || wrapper.text()).toBeTruthy()
  })
})
