// logv/src/api/logs.spec.js — logs API 模块测试基线
//
// 覆盖 http 实例暴露 correctness + searchLogs 调用参数正确性。
// 使用 vi.mock 拦截 ./http 模块，验证 api logs 层对 http 层的委托调用。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// 模拟 http 模块（必须在被测模块导入前完成）
const mockPost = vi.fn()
const mockGet = vi.fn()

vi.mock('./http', () => ({
  default: {
    post: (...args) => mockPost(...args),
    get: (...args) => mockGet(...args),
  },
}))

// 动态导入被测模块（确保 mock 已生效）
const logsApi = await import('./logs')

describe('logs API — http 实例委托', () => {
  beforeEach(() => {
    mockPost.mockClear()
    mockGet.mockClear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('queryLogClients 正确调用 http.get(/logs/queryClients)', () => {
    logsApi.queryLogClients()
    expect(mockGet).toHaveBeenCalledWith('/logs/queryClients')
  })

  it('queryItemsByClient 携带 client_id 查询参数', () => {
    logsApi.queryItemsByClient(42)
    expect(mockGet).toHaveBeenCalledWith('/logs/queryItems', { params: { client_id: 42 } })
  })

  it('queryLogs 使用 POST + blob responseType', () => {
    const payload = { item: 1, date: '20260101', key: 'ERROR' }
    logsApi.queryLogs(payload)
    expect(mockPost).toHaveBeenCalledWith('/logs/query', payload, { responseType: 'blob' })
  })

  it('searchLogs 正确调用 http.post(/logs/search, payload)', () => {
    const payload = { item: 1, date: '20260101', key: 'timeout', pageNo: 1, pageSize: 50 }
    logsApi.searchLogs(payload)
    expect(mockPost).toHaveBeenCalledWith('/logs/search', payload)
    // 确认仅传两个参数（不附加 config）
    expect(mockPost.mock.calls[0].length).toBe(2)
  })

  it('traceLogs 正确调用 http.post(/logs/trace, payload)', () => {
    const payload = { traceId: 'abc123', date: '20260101' }
    logsApi.traceLogs(payload)
    expect(mockPost).toHaveBeenCalledWith('/logs/trace', payload)
  })
})
