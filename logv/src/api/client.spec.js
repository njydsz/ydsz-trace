// logv/src/api/client.spec.js — client API 模块测试
//
// 覆盖：clientApi 各函数正确调用 http（mock http 模块）。
// 验证分页 / 增删改查接口的路径与参数。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const mockPost = vi.fn()
const mockGet = vi.fn()

vi.mock('./http', () => ({
  default: {
    post: (...args) => mockPost(...args),
    get: (...args) => mockGet(...args),
  },
}))

const clientApi = await import('./client')

describe('client API — http 调用委托', () => {
  beforeEach(() => {
    mockPost.mockClear()
    mockGet.mockClear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('queryClients 分页查询传递 page/limit 参数', () => {
    clientApi.queryClients(2, 20)
    expect(mockGet).toHaveBeenCalledWith('/client/queryPage', { params: { page: 2, limit: 20 } })
  })

  it('queryClients 使用默认参数 page=1, limit=10', () => {
    clientApi.queryClients()
    expect(mockGet).toHaveBeenCalledWith('/client/queryPage', { params: { page: 1, limit: 10 } })
  })

  it('queryAllClients 查询全部客户端（无参数）', () => {
    clientApi.queryAllClients()
    expect(mockGet).toHaveBeenCalledWith('/client/queryAll')
  })

  it('getClient 按 id 查询单个客户端', () => {
    clientApi.getClient(42)
    expect(mockGet).toHaveBeenCalledWith('/client/query', { params: { id: 42 } })
  })

  it('addClient 通过 POST 新增客户端', () => {
    const payload = { ip: '10.0.0.1', port: 9090 }
    clientApi.addClient(payload)
    expect(mockPost).toHaveBeenCalledWith('/client/add', payload)
  })

  it('updateClient 通过 POST 更新客户端', () => {
    const payload = { id: 1, ip: '10.0.0.2', port: 9091 }
    clientApi.updateClient(payload)
    expect(mockPost).toHaveBeenCalledWith('/client/update', payload)
  })

  it('deleteClient 通过 POST 传递 id 删除客户端', () => {
    clientApi.deleteClient(10)
    expect(mockPost).toHaveBeenCalledWith('/client/delete', { id: 10 })
  })

  it('changeClientStatus 通过 POST 传递 id 和 status', () => {
    clientApi.changeClientStatus(5, 1)
    expect(mockPost).toHaveBeenCalledWith('/client/changeStatus', { id: 5, status: 1 })
  })
})
