// 仪表盘 E2E 用例：页面加载、骨架屏、快捷操作入口。
//
// 运行方式：npx playwright test e2e/dashboard.spec.js
import { test, expect } from '@playwright/test'
import { login, navigateTo } from './utils/test-helpers'

test.describe('仪表盘页面', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('应加载仪表盘并展示欢迎与问候信息', async ({ page }) => {
    await navigateTo(page, '/dashboard')

    await expect(page.getByTestId('dashboard-greeting')).toBeVisible()
    await expect(page.getByTestId('dashboard-greeting-text')).toContainText('欢迎回来')
    await expect(page.getByTestId('dashboard-welcome-title')).toContainText('Ydsz Trace')
    await expect(page.getByTestId('dashboard-welcome-desc')).toBeVisible()
  })

  test('洞察看板区域应展示（骨架屏或内容）', async ({ page }) => {
    await navigateTo(page, '/dashboard')

    // 数据加载中显示骨架屏；加载完成后骨架屏消失、洞察组件出现。
    const skeleton = page.getByTestId('dashboard-insight-skeleton')
    const insight = page.getByTestId('dashboard-insight')

    // 两者至少出现其一（取决于请求完成时机）。
    await expect(skeleton.or(insight).first()).toBeVisible()
    // 骨架屏与洞察区互斥，不应同时存在。
    const bothVisible =
      (await skeleton.isVisible().catch(() => false)) &&
      (await insight.isVisible().catch(() => false))
    expect(bothVisible).toBe(false)
  })

  test('快捷操作入口应完整并可点击跳转', async ({ page }) => {
    await navigateTo(page, '/dashboard')

    const quickActions = page.getByTestId('dashboard-quick-actions')
    await expect(quickActions).toBeVisible()

    const entries = [
      ['quick-action-conversation', '/conversation'],
      ['quick-action-logs', '/logs'],
      ['quick-action-trace', '/trace'],
    ]
    for (const [testId, path] of entries) {
      const btn = page.getByTestId(testId)
      await expect(btn).toBeVisible()
      await Promise.all([
        page.waitForURL(`**${path}`, { timeout: 15_000 }).catch(() => {}),
        btn.click(),
      ])
      await expect(page).toHaveURL(new RegExp(path.replace('/', '\\/')))
      // 返回仪表盘继续下一个入口校验。
      await navigateTo(page, '/dashboard')
    }
  })
})
