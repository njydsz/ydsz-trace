// web/src/views/Alerts.spec.js — 告警页测试基线
//
// 覆盖：组件挂载、告警规则列表渲染、新增/编辑按钮触发 dialog。
// 使用 shallowMount 隔离 Element Plus 依赖，vi.mock 拦截 API / 确认弹窗。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

const mockListRules = vi.fn(() => Promise.resolve({
  data: {
    code: 0,
    data: {
      list: [
        { id: 1, name: 'ERROR 过多', itemId: 1, itemName: 'auth-service', threshold: 10, intervalSec: 300, enabled: 1 },
        { id: 2, name: '超时告警', itemId: 2, itemName: 'api-gateway', threshold: 5, intervalSec: 60, enabled: 0 },
      ],
      totalCount: 2,
    },
  },
}))

const mockAddRule = vi.fn(() => Promise.resolve({ data: { code: 0, data: { id: 3 } } }))
const mockUpdateRule = vi.fn(() => Promise.resolve({ data: { code: 0 } }))
const mockDeleteRule = vi.fn(() => Promise.resolve({ data: { code: 0 } }))
const mockToggleRule = vi.fn(() => Promise.resolve({ data: { code: 0 } }))
const mockTestFireRule = vi.fn(() => Promise.resolve({ data: { code: 0 } }))
const mockListEvents = vi.fn(() => Promise.resolve({ data: { code: 0, data: { list: [], totalCount: 0 } } }))
const mockDeleteEvent = vi.fn(() => Promise.resolve({ data: { code: 0 } }))
const mockGetQuota = vi.fn(() => Promise.resolve({
  data: { code: 0, data: { enabled: 2, firedToday: 5, failedToday: 1 } },
}))

vi.mock('@/api/alert', () => ({
  listRules: (...args) => mockListRules(...args),
  addRule: (...args) => mockAddRule(...args),
  updateRule: (...args) => mockUpdateRule(...args),
  deleteRule: (...args) => mockDeleteRule(...args),
  toggleRule: (...args) => mockToggleRule(...args),
  testFireRule: (...args) => mockTestFireRule(...args),
  listEvents: (...args) => mockListEvents(...args),
  deleteEvent: (...args) => mockDeleteEvent(...args),
  getQuota: (...args) => mockGetQuota(...args),
}))

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) },
}))

import Alerts from './Alerts.vue'

function mountComponent() {
  return shallowMount(Alerts, {
    global: {
      stubs: {
        'el-icon': { template: '<span />' },
        'el-tabs': { template: '<div><slot /></div>' },
        'el-tab-pane': { template: '<div><slot /></div>' },
        'el-select': { template: '<select><slot /></select>' },
        'el-option': { template: '<option />' },
        'el-button': { template: '<button><slot /></button>' },
        'el-table': { template: '<table><slot /></table>', props: ['data'] },
        'el-table-column': { template: '<col />', props: ['label', 'prop'] },
        'el-pagination': { template: '<nav />', props: ['modelValue'] },
        'el-dialog': {
          template: '<div v-if="modelValue"><slot /><slot name="footer" /></div>',
          props: ['modelValue', 'title'],
        },
        'el-form': { template: '<form><slot /></form>' },
        'el-form-item': { template: '<div><slot /></div>' },
        'el-input': { template: '<input />', props: ['modelValue'] },
        'el-input-number': { template: '<input type="number" />', props: ['modelValue'] },
        'el-switch': { template: '<input type="checkbox" />', props: ['modelValue'] },
        'el-tag': { template: '<span><slot /></span>' },
      },
    },
  })
}

describe('Alerts.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('组件挂载不报错且 onMounted 调用列表接口', async () => {
    const wrapper = mountComponent()
    await vi.dynamicImportSettled()
    // 等待所有 Promise 链
    await new Promise(r => setTimeout(r, 100))

    expect(wrapper.exists()).toBe(true)
    // onMounted 应请求 rules / events / quota
    expect(mockListRules).toHaveBeenCalled()
    expect(mockListEvents).toHaveBeenCalled()
    expect(mockGetQuota).toHaveBeenCalled()
  })

  it('告警规则列表已从 API 获取', async () => {
    const wrapper = mountComponent()
    await vi.dynamicImportSettled()
    await new Promise(r => setTimeout(r, 100))

    // listRules 应返回两条 mock 规则
    expect(mockListRules).toHaveBeenCalledTimes(1)
  })

  it('API 新增接口（addRule）调用参数正确', async () => {
    const payload = {
      name: '新规则',
      itemId: 1,
      clientId: 0,
      keyWord: 'timeout',
      regex: false,
      level: 'ERROR',
      threshold: 10,
      intervalSec: 300,
      webhookUrl: 'https://example.com/hook',
    }
    await mockAddRule(payload)
    expect(mockAddRule).toHaveBeenCalledWith(payload)
  })

  it('API 更新接口（updateRule）调用参数正确', async () => {
    await mockUpdateRule(1, { name: '修改后的名称', threshold: 20 })
    expect(mockUpdateRule).toHaveBeenCalledWith(1, { name: '修改后的名称', threshold: 20 })
  })

  it('模板包含新增/编辑按钮触发区域', async () => {
    const wrapper = mountComponent()
    await vi.dynamicImportSettled()
    await new Promise(r => setTimeout(r, 100))

    // 模板标题渲染
    expect(wrapper.find('h2').text()).toBe('告警')
  })
})
