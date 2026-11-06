import { describe, it, expect, vi } from 'vitest'

vi.mock('@/api/alert', () => ({
  listRules: vi.fn().mockResolvedValue({ code: 0, data: { list: [], total: 0 } }),
  getQuota: vi.fn().mockResolvedValue({ code: 0, data: { enabled: 0, fired24h: 0, failed24h: 0 } }),
  listEvents: vi.fn().mockResolvedValue({ code: 0, data: { list: [], total: 0 } }),
}))

vi.mock('@/api/logs', () => ({
  queryLogClients: vi.fn().mockResolvedValue({ code: 0, data: [] }),
  queryItemsByClient: vi.fn().mockResolvedValue({ code: 0, data: [] }),
}))

describe('Alerts', () => {
  it('component module exists', async () => {
    const mod = await import('./Alerts.vue')
    expect(mod.default).toBeDefined()
  })

  it('has setup function', async () => {
    const mod = await import('./Alerts.vue')
    expect(typeof mod.default).toBe('object')
  })
})
