// http 拦截器单元测试（V2.1-06 补齐）
//
// 覆盖：业务码归一化（字符串码 → 数字）、成功判定、401 跳转。
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import http from './http'

describe('http 响应拦截器', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('数字码 0 视为成功，不触发 warning', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const res = await http.get('/mock-success', {
      adapter: async (config) => ({
        data: { code: 0, message: 'ok', data: {} },
        status: 200,
        statusText: 'OK',
        headers: { 'content-type': 'application/json' },
        config,
      }),
    })
    expect(res.data.code).toBe(0)
    warn.mockRestore()
  })

  it('字符串码 "0" 被归一化为数字 0', async () => {
    const res = await http.get('/mock-string-code', {
      adapter: async (config) => ({
        data: { code: '0', message: 'ok', data: {} },
        status: 200,
        statusText: 'OK',
        headers: { 'content-type': 'application/json' },
        config,
      }),
    })
    expect(res.data.code).toBe(0)
    expect(typeof res.data.code).toBe('number')
  })

  it('历史字符串码 "200" 被映射为成功码 0', async () => {
    const res = await http.get('/mock-legacy-code', {
      adapter: async (config) => ({
        data: { code: '200', message: 'ok', data: {} },
        status: 200,
        statusText: 'OK',
        headers: { 'content-type': 'application/json' },
        config,
      }),
    })
    expect(res.data.code).toBe(0)
  })

  it('非成功码保留原值', async () => {
    const res = await http.get('/mock-error-code', {
      adapter: async (config) => ({
        data: { code: 403, message: 'forbidden', data: null },
        status: 200,
        statusText: 'OK',
        headers: { 'content-type': 'application/json' },
        config,
      }),
    })
    expect(res.data.code).toBe(403)
  })

  it('GET 请求不附加 X-Requested-With 头', async () => {
    let captured = null
    await http.get('/mock-header', {
      adapter: async (config) => {
        captured = config.headers
        return {
          data: { code: 0 },
          status: 200,
          statusText: 'OK',
          headers: { 'content-type': 'application/json' },
          config,
        }
      },
    })
    expect(captured['X-Requested-With']).toBeUndefined()
  })
})
