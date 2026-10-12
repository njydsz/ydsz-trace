// web/src/views/ConversationWorkbench.spec.js — 对话式工作台测试基线
//
// 覆盖：组件挂载、输入文字按 Enter 发送、结果消息气泡渲染、快捷预设按钮更新输入。
// 使用 shallowMount 隔离 Element Plus 依赖，vi.mock 拦截 searchLogs API。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

// 模拟 API 依赖
vi.mock('@/api/logs', () => ({
  searchLogs: vi.fn(() => Promise.resolve({
    data: { code: '200', data: { list: [], total: 0, truncated: false, taskNo: '' } },
  })),
}))

// 模拟 vue-router
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

// 模拟 element-plus
vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

import ConversationWorkbench from './ConversationWorkbench.vue'
import { searchLogs } from '@/api/logs'

function mountComponent() {
  return shallowMount(ConversationWorkbench, {
    global: {
      stubs: {
        'el-input': { template: '<textarea />', props: ['modelValue'] },
        'el-button': { template: '<button><slot /></button>' },
        'el-icon': { template: '<span />' },
        'el-card': { template: '<div><slot /></div>' },
        'el-tag': { template: '<span><slot /></span>' },
        'el-alert': { template: '<div />' },
      },
    },
  })
}

describe('ConversationWorkbench.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('组件挂载不报错', () => {
    const wrapper = mountComponent()
    expect(wrapper.exists()).toBe(true)
  })

  it('初始状态：消息列表为空或仅包含欢迎消息', () => {
    const wrapper = mountComponent()
    // 组件初始时暴露的 messages 应为空（欢迎消息在模板内渲染）
    expect(wrapper.vm.messages).toBeDefined()
    expect(Array.isArray(wrapper.vm.messages)).toBe(true)
  })

  it('输入文字后按 Enter 触发发送并构造请求', async () => {
    searchLogs.mockResolvedValueOnce({
      data: {
        code: '200',
        data: {
          list: [
            { line: '[ERROR] 2026-01-01 timeout connecting to upstream' },
          ],
          total: 1,
          truncated: false,
          taskNo: '',
        },
      },
    })
    const wrapper = mountComponent()
    wrapper.vm.inputText = '今天 ERROR 日志'

    await wrapper.vm.handleSend()
    await new Promise(r => setTimeout(r, 100))

    // 应调用 searchLogs 且参数携带 level=ERROR
    expect(searchLogs).toHaveBeenCalled()
    const callArgs = searchLogs.mock.calls[0][0]
    expect(callArgs.level).toBe('ERROR')
    // 用户消息应已加入列表
    expect(wrapper.vm.messages.length).toBeGreaterThanOrEqual(1)
    expect(wrapper.vm.messages[0].role).toBe('user')
    expect(wrapper.vm.messages[0].content).toBe('今天 ERROR 日志')
  })

  it('快捷预设按钮点击后更新输入框内容', () => {
    const wrapper = mountComponent()
    expect(wrapper.vm.inputText).toBe('')

    // 模拟点击第一个快捷预设
    wrapper.vm.handleQuickPreset({ label: '近 1 小时 ERROR', query: '近 1 小时 ERROR 日志' })
    expect(wrapper.vm.inputText).toBe('近 1 小时 ERROR 日志')

    // 模拟点击第二个快捷预设
    wrapper.vm.handleQuickPreset({ label: '今天 WARN', query: '今天 WARN 级别日志' })
    expect(wrapper.vm.inputText).toBe('今天 WARN 级别日志')
  })

  it('搜索结果返回后渲染助手消息气泡', async () => {
    searchLogs.mockResolvedValueOnce({
      data: {
        code: '200',
        data: {
          list: [
            { line: '[ERROR] connection refused' },
            { line: '[ERROR] socket read timeout' },
          ],
          total: 2,
          truncated: false,
          taskNo: '',
        },
      },
    })

    const wrapper = mountComponent()
    wrapper.vm.inputText = '今天 ERROR'
    await wrapper.vm.handleSend()
    await new Promise(r => setTimeout(r, 100))

    // 应有一条用户消息 + 一条助手消息
    expect(wrapper.vm.messages.length).toBe(2)
    const assistantMsg = wrapper.vm.messages[1]
    expect(assistantMsg.role).toBe('assistant')
    expect(assistantMsg.type).toBe('chart')
    expect(assistantMsg.resultTotal).toBe(2)
    expect(assistantMsg.rows.length).toBe(2)
  })

  it('搜索无结果时正确展示空结果提示', async () => {
    searchLogs.mockResolvedValueOnce({
      data: {
        code: '200',
        data: { list: [], total: 0, truncated: false, taskNo: '' },
      },
    })

    const wrapper = mountComponent()
    wrapper.vm.inputText = '查一部不存在的日志'
    await wrapper.vm.handleSend()
    await new Promise(r => setTimeout(r, 100))

    // 应有一条助手消息且 matchCount=0
    expect(wrapper.vm.messages.length).toBe(2)
    const assistantMsg = wrapper.vm.messages[1]
    expect(assistantMsg.explanation.matchCount).toBe(0)
  })

  it('解析自然语言：正确提取级别和时间关键词', () => {
    const wrapper = mountComponent()
    const result = wrapper.vm.parseNaturalLanguage('近 1 小时 ERROR 日志')

    expect(result.level).toBe('ERROR')
    expect(result.startTime).toBeTruthy()
    expect(result.endTime).toBeTruthy()
  })

  it('解析自然语言：提取引号内文本作为查询关键词', () => {
    const wrapper = mountComponent()
    const result = wrapper.vm.parseNaturalLanguage('查找包含 "NullPointerException" 的日志')

    expect(result.key).toBe('NullPointerException')
  })

  it('搜索失败时展示错误消息和降级建议', async () => {
    searchLogs.mockRejectedValueOnce(new Error('Network error'))

    const wrapper = mountComponent()
    wrapper.vm.inputText = '今天 ERROR'
    await wrapper.vm.handleSend()
    await new Promise(r => setTimeout(r, 100))

    // 应有一条错误类型的助手消息
    const errorMsg = wrapper.vm.messages.find(m => m.type === 'error')
    expect(errorMsg).toBeTruthy()
    expect(errorMsg.role).toBe('assistant')
    expect(errorMsg.fallbackQuery).toBe('ERROR')
  })
})
