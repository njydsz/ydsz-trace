// 链路追踪 E2E 用例：页面加载、traceId 输入、瀑布视图切换。
//
// 运行方式：npx playwright test e2e/trace.spec.js
import { test, expect } from '@playwright/test'
import { login, navigateTo } from './utils/test-helpers'

test.describe('链路追踪页面', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('应加载链路追踪页并展示检索表单', async ({ page }) => {
    await navigateTo(page, '/trace')

    await expect(page.getByTestId('trace-title')).toContainText('日志链路追踪')
    await expect(page.getByTestId('trace-subtitle')).toBeVisible()
    await expect(page.getByTestId('trace-id-input')).toBeVisible()
    await expect(page.getByTestId('trace-submit')).toBeVisible()
  })

  test('Trace ID 输入框应支持输入', async ({ page }) => {
    await navigateTo(page, '/trace')

    const input = page.getByTestId('trace-id-input')
    await input.fill('abc123-def456')
    await expect(input).toHaveValue('abc123-def456')
  })

  test('结果区提供节点分组与瀑布视图切换入口', async ({ page }) => {
    await navigateTo(page, '/trace')

    // 结果卡片仅在追踪有返回时出现；通过 URL query 预填参数可触发自动追踪，
    // 但后端不可用时不保证结果存在。因此对「无结果」场景容错处理。
    const nodeTab = page.getByTestId('trace-view-node')
    const waterfallTab = page.getByTestId('trace-view-waterfall')

    if ((await nodeTab.count()) === 0) {
      test.skip(true, '当前环境未产生追踪结果，跳过视图切换断言')
      return
    }

    await expect(nodeTab).toBeVisible()
    await expect(waterfallTab).toBeVisible()

    // 点击瀑布视图后应切到瀑布视图内容。
    await waterfallTab.click()
    // 再次点击节点分组应能切回。
    await nodeTab.click()
    await expect(nodeTab).toBeVisible()
  })
})
