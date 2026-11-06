import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createWebHistory } from 'vue-router'
import { createPinia } from 'pinia'
import Dashboard from './Dashboard.vue'

vi.mock('@/api/logs', () => ({
  dashboardInsight: vi.fn().mockResolvedValue({
    code: 0,
    data: {
      templateTrends: [],
      errorTimeline: [],
      serviceHealth: [],
    },
  }),
}))

describe('Dashboard', () => {
  let wrapper

  beforeEach(() => {
    const router = createRouter({
      history: createWebHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }],
    })
    wrapper = mount(Dashboard, {
      global: {
        plugins: [createPinia(), router],
      },
    })
  })

  it('renders dashboard header', () => {
    expect(wrapper.text()).toBeTruthy()
  })

  it('has quick action links', () => {
    const links = wrapper.findAll('a')
    expect(links.length).toBeGreaterThan(0)
  })
})
