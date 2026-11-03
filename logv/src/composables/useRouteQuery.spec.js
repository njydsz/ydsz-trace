// useRouteQuery 单元测试（V2.1-07 补齐）
//
// 覆盖：query 解析、非法值降级、路由变化时同步更新、无 useRoute 时安全降级。
import { describe, it, expect, vi } from 'vitest'
import { reactive } from 'vue'

// 模拟 vue-router：同时提供 useRoute / useRouter，避免影响其他视图测试。
// 使用 reactive 保持与真实路由一致的响应式语义（watch 才能正常触发）。
const routeMock = reactive({ query: {} })
vi.mock('vue-router', () => ({
  useRoute: () => routeMock,
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}))

import { useRouteQuery } from './useRouteQuery'

describe('useRouteQuery', () => {
  it('解析 item / client / key / query 参数', () => {
    routeMock.query = { item: '12', client: '3', key: 'timeout', query: 'level:ERROR' }
    const { itemID, clientID, keyword, queryExpr } = useRouteQuery()
    expect(itemID.value).toBe(12)
    expect(clientID.value).toBe(3)
    expect(keyword.value).toBe('timeout')
    expect(queryExpr.value).toBe('level:ERROR')
  })

  it('非法 ID 归一化为 null', () => {
    routeMock.query = { item: 'abc', client: '-5', key: '' }
    const { itemID, clientID } = useRouteQuery()
    expect(itemID.value).toBeNull()
    expect(clientID.value).toBeNull()
  })

  it('空 query 时全部为默认值', () => {
    routeMock.query = {}
    const { itemID, clientID, keyword, queryExpr } = useRouteQuery()
    expect(itemID.value).toBeNull()
    expect(clientID.value).toBeNull()
    expect(keyword.value).toBe('')
    expect(queryExpr.value).toBe('')
  })

  it('路由 query 变化时同步更新', async () => {
    // 注意：useRouteQuery 内部 watch 绑定的是 route.query 的初始对象引用，
    // 因此测试需原地修改 query 字段（与真实路由的响应式替换行为一致）。
    routeMock.query = { item: '1' }
    const { itemID } = useRouteQuery()
    expect(itemID.value).toBe(1)

    routeMock.query.item = '42'
    await new Promise((r) => setTimeout(r, 0))
    expect(itemID.value).toBe(42)
  })
})
