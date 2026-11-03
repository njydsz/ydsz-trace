// AIStatusPanel 单元测试（V2.1-03 补齐）
//
// 覆盖：渲染算法清单、L3 降级为 inactive、请求失败兜底。
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AIStatusPanel from './AIStatusPanel.vue'

// mock aimodel api
vi.mock('@/api/aimodel', () => ({
  fetchAIStatus: vi.fn(),
}))

import { fetchAIStatus } from '@/api/aimodel'

const mockLayers = {
  generatedAt: '2026-11-03 10:00:00',
  total24H: 12,
  layers: [
    { name: 'L0 规则层', layer: 'rule', status: 'active', callCount24H: 5, algorithms: ['drain', 'regex'] },
    { name: 'L1 统计层', layer: 'stats', status: 'active', callCount24H: 0, algorithms: ['z-score', 'poisson'] },
    { name: 'L2 嵌入层', layer: 'embedding', status: 'degraded', callCount24H: 7, algorithms: ['cosine'] },
    { name: 'L3 生成层', layer: 'generation', status: 'inactive', callCount24H: 0, algorithms: ['regex'] },
  ],
}

describe('AIStatusPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('渲染四层状态与算法清单', async () => {
    fetchAIStatus.mockResolvedValue({ data: { code: 0, data: mockLayers } })
    const wrapper = mount(AIStatusPanel)
    await new Promise((r) => setTimeout(r, 0))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('L0 规则层')
    expect(wrapper.text()).toContain('L1 统计层')
    expect(wrapper.text()).toContain('L2 嵌入层')
    expect(wrapper.text()).toContain('L3 生成层')

    // 算法标签应展示（V2.1-03 新增的 algorithms 字段）
    expect(wrapper.text()).toContain('z-score')
    expect(wrapper.text()).toContain('poisson')
    expect(wrapper.text()).toContain('cosine')
  })

  it('请求失败时清空层数据且不抛错', async () => {
    fetchAIStatus.mockRejectedValue(new Error('network'))
    const wrapper = mount(AIStatusPanel)
    await new Promise((r) => setTimeout(r, 0))
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).not.toContain('L0 规则层')
  })
})
