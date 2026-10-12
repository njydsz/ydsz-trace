// web/src/api/alert.spec.js — alert API 模块测试
//
// 覆盖：alertApi 各函数正确调用 http（mock http 模块）。
// 使用 vi.mock 拦截 ./http 模块，验证 API 层的委托调用。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const mockPost = vi.fn()
const mockGet = vi.fn()
const mockPut = vi.fn()
const mockDelete = vi.fn()

vi.mock('./http', () => ({
  default: {
    post: (...args) => mockPost(...args),
    get: (...args) => mockGet(...args),
    put: (...args) => mockPut(...args),
    delete: (...args) => mockDelete(...args),
  },
}))

const alertApi = await import('./alert')

describe('alert API — http 调用委托', () => {
  beforeEach(() => {
    mockPost.mockClear()
    mockGet.mockClear()
    mockPut.mockClear()
    mockDelete.mockClear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('listRules 正确调用 http.get(/logs/alerts/rules)', () => {
    alertApi.listRules({ page: 1, limit: 10 })
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/rules', { params: { page: 1, limit: 10 } })
  })

  it('listRules 无参数时传递空对象', () => {
    alertApi.listRules()
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/rules', { params: {} })
  })

  it('getRule 携带 id 调用 http.get', () => {
    alertApi.getRule(42)
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/rules/42')
  })

  it('addRule 通过 http.post 提交 payload', () => {
    const payload = { name: '新规则', itemId: 1, threshold: 5 }
    alertApi.addRule(payload)
    expect(mockPost).toHaveBeenCalledWith('/logs/alerts/rules', payload)
  })

  it('updateRule 使用 http.put 更新', () => {
    alertApi.updateRule(1, { name: '修改后的名称' })
    expect(mockPut).toHaveBeenCalledWith('/logs/alerts/rules/1', { name: '修改后的名称' })
  })

  it('deleteRule 调用 http.delete', () => {
    alertApi.deleteRule(99)
    expect(mockDelete).toHaveBeenCalledWith('/logs/alerts/rules/99')
  })

  it('toggleRule 调用 http.post 传递 id 和 enabled', () => {
    alertApi.toggleRule(1, true)
    expect(mockPost).toHaveBeenCalledWith('/logs/alerts/rules/toggle', { id: 1, enabled: true })
  })

  it('testFireRule 调用 http.post 传递 id', () => {
    alertApi.testFireRule(5)
    expect(mockPost).toHaveBeenCalledWith('/logs/alerts/rules/test', { id: 5 })
  })

  it('listEvents 正确调用 http.get(/logs/alerts/events)', () => {
    alertApi.listEvents({ ruleId: 1 })
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/events', { params: { ruleId: 1 } })
  })

  it('deleteEvent 调用 http.delete', () => {
    alertApi.deleteEvent(7)
    expect(mockDelete).toHaveBeenCalledWith('/logs/alerts/events/7')
  })

  it('getQuota 调用 http.get(/logs/alerts/quota)', () => {
    alertApi.getQuota()
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/quota')
  })
})
