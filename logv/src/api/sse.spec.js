// logv/src/api/sse.spec.js — 安全 SSE 连接封装单元测试。
//
// 覆盖：SSE 事件解析、401 响应处理（登录态清空+跳转）、指数退避重连。
// 使用 mock fetch 替代真实网络请求。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createSSE, parseSSEEvent } from '@/api/sse'

// 模拟 window.location（保留 AbortController 等原生全局对象）
const mockLocation = {
  pathname: '/logs/tail',
  href: '',
}
const originalWindow = globalThis.window
beforeEach(() => {
  // 仅覆盖 location，保留其他全局对象（AbortController、Promise 等）
  Object.defineProperty(globalThis, 'window', {
    value: { ...originalWindow, location: mockLocation },
    writable: true,
    configurable: true,
  })
})
afterAll(() => {
  // 恢复原始 window
  Object.defineProperty(globalThis, 'window', {
    value: originalWindow,
    writable: true,
    configurable: true,
  })
})

// 模拟 useAuth store
const mockClear = vi.fn()
vi.mock('@/store/auth', () => ({
  useAuth: () => ({ clear: mockClear }),
}))

// 拦截 fetch
const originalFetch = globalThis.fetch

function installMockFetch(fn) {
  globalThis.fetch = fn
}

describe('parseSSEEvent', () => {
  it('解析标准 SSE 块：提取 event 和 data', () => {
    const block = 'event: progress\ndata: 50%'
    const result = parseSSEEvent(block)
    expect(result.event).toBe('progress')
    expect(result.data).toBe('50%')
  })

  it('解析多行 data：用换行符连接', () => {
    const block = 'data: line1\ndata: line2\ndata: line3'
    const result = parseSSEEvent(block)
    expect(result.event).toBe('message')
    expect(result.data).toBe('line1\nline2\nline3')
  })

  it('空 data 返回 null', () => {
    const block = 'event: ping'
    expect(parseSSEEvent(block)).toBeNull()
  })
})

describe('createSSE 401 处理', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockClear.mockReset()
    mockLocation.href = ''
    mockLocation.pathname = '/logs/tail'
  })

  afterEach(() => {
    vi.useRealTimers()
    globalThis.fetch = originalFetch
  })

  it('401 响应时清空登录态并跳转登录页', async () => {
    installMockFetch(() =>
      Promise.resolve({ status: 401, ok: false, body: null })
    )

    createSSE('/logs/tail', { method: 'POST', body: {} })
    // 等待 fetch Promise 回调
    await vi.advanceTimersByTimeAsync(0)

    expect(mockClear).toHaveBeenCalled()
    expect(mockLocation.href).toContain('/login')
  })

  it('401 时不调用 onMessage 或 onError', async () => {
    const onMessage = vi.fn()
    const onError = vi.fn()

    installMockFetch(() =>
      Promise.resolve({ status: 401, ok: false, body: null })
    )

    createSSE('/logs/tail', {
      method: 'POST',
      body: {},
      onMessage,
      onError,
    })
    await vi.advanceTimersByTimeAsync(0)

    expect(onMessage).not.toHaveBeenCalled()
    expect(onError).not.toHaveBeenCalled()
  })

  it('非 401 错误时触发重连（指数退避）', async () => {
    const onError = vi.fn()
    let callCount = 0
    installMockFetch(() => {
      callCount++
      return Promise.resolve({ status: 502, ok: false, body: null })
    })

    createSSE('/logs/tail', {
      method: 'POST',
      body: {},
      maxRetries: 2,
      retryInterval: 100,
      onError,
    })

    // 触发 502 → 第一次重连（100ms）
    await vi.advanceTimersByTimeAsync(50)
    await vi.advanceTimersByTimeAsync(150)

    // 第二次重连（200ms）
    await vi.advanceTimersByTimeAsync(250)

    // 超过 maxRetries → onError 被调用
    expect(callCount).toBeGreaterThan(1)
    expect(onError).toHaveBeenCalled()
  })
})

// 注：SSE 消息流的实时接收依赖于浏览器原生 ReadableStream + getReader API，
// 在 Node.js 测试环境中难以完整 mock。消息解析与事件分发逻辑已通过
// parseSSEEvent 单元测试和后端 SSE 安全测试（sse_security_test.go）覆盖。
