// alert.js API 层单元测试
//
// 覆盖所有 alert API 方法的 HTTP 调用：
//   - listRules / listEvents / getQuota 使用 GET
//   - addRule / updateRule / deleteRule / toggleRule / testFireRule / deleteEvent 使用 POST/DELETE
//   - 请求参数正确传递
import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock http 模块
const mockGet = vi.fn()
const mockPost = vi.fn()
const mockPut = vi.fn()
const mockDelete = vi.fn()

vi.mock('./http', () => ({
  default: {
    get: (...args) => mockGet(...args),
    post: (...args) => mockPost(...args),
    put: (...args) => mockPut(...args),
    delete: (...args) => mockDelete(...args),
  },
}))

import {
  listRules,
  getRule,
  addRule,
  updateRule,
  deleteRule,
  toggleRule,
  testFireRule,
  listEvents,
  deleteEvent,
  getQuota,
} from './alert'

describe('alert API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('listRules: GET /logs/alerts/rules with pagination params', async () => {
    await listRules({ pageNo: 1, pageSize: 20 })
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/rules', { params: { pageNo: 1, pageSize: 20 } })
  })

  it('listRules: 默认空参数', async () => {
    await listRules()
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/rules', { params: {} })
  })

  it('getRule: GET /logs/alerts/rules/:id', async () => {
    await getRule(42)
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/rules/42')
  })

  it('addRule: POST /logs/alerts/rules with payload', async () => {
    const payload = { name: 'test', itemId: 1, webhookUrl: 'http://hook' }
    await addRule(payload)
    expect(mockPost).toHaveBeenCalledWith('/logs/alerts/rules', payload)
  })

  it('updateRule: PUT /logs/alerts/rules/:id with payload', async () => {
    const payload = { name: 'updated' }
    await updateRule(1, payload)
    expect(mockPut).toHaveBeenCalledWith('/logs/alerts/rules/1', payload)
  })

  it('deleteRule: DELETE /logs/alerts/rules/:id', async () => {
    await deleteRule(5)
    expect(mockDelete).toHaveBeenCalledWith('/logs/alerts/rules/5')
  })

  it('toggleRule: POST /logs/alerts/rules/toggle', async () => {
    await toggleRule(1, 1)
    expect(mockPost).toHaveBeenCalledWith('/logs/alerts/rules/toggle', { id: 1, enabled: 1 })
  })

  it('testFireRule: POST /logs/alerts/rules/test', async () => {
    await testFireRule(1)
    expect(mockPost).toHaveBeenCalledWith('/logs/alerts/rules/test', { id: 1 })
  })

  it('listEvents: GET /logs/alerts/events with status filter', async () => {
    await listEvents({ pageNo: 1, pageSize: 10, status: 'ok' })
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/events', {
      params: { pageNo: 1, pageSize: 10, status: 'ok' },
    })
  })

  it('deleteEvent: DELETE /logs/alerts/events/:id', async () => {
    await deleteEvent(10)
    expect(mockDelete).toHaveBeenCalledWith('/logs/alerts/events/10')
  })

  it('getQuota: GET /logs/alerts/quota', async () => {
    await getQuota()
    expect(mockGet).toHaveBeenCalledWith('/logs/alerts/quota')
  })
})
