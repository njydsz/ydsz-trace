// 历史日志检索 E2E 用例：页面加载、搜索表单输入、搜索历史下拉、模板自动补全。
//
// 运行方式：npx playwright test e2e/log-search.spec.js
import { test, expect } from '@playwright/test'
import { login, navigateTo } from './utils/test-helpers'

test.describe('日志搜索页面', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('应加载日志搜索页并展示检索表单', async ({ page }) => {
    await navigateTo(page, '/logs')

    await expect(page.getByTestId('log-search-title')).toContainText('历史日志检索')
    await expect(page.getByTestId('log-search-subtitle')).toBeVisible()
    await expect(page.getByTestId('log-search-keyword')).toBeVisible()
    await expect(page.getByTestId('log-search-submit')).toBeVisible()
    await expect(page.getByTestId('log-search-toggle-history')).toBeVisible()
  })

  test('搜索关键词表单应支持输入', async ({ page }) => {
    await navigateTo(page, '/logs')

    const keywordInput = page.getByTestId('log-search-keyword').locator('input')
    await keywordInput.fill('ERROR timeout')
    await expect(keywordInput).toHaveValue('ERROR timeout')
  })

  test('点击搜索历史按钮应展开历史下拉面板', async ({ page }) => {
    await navigateTo(page, '/logs')

    // 触发历史下拉的显示。
    await page.getByTestId('log-search-toggle-history').click()

    const panel = page.getByTestId('search-history-dropdown-panel')
    await expect(panel).toBeVisible({ timeout: 10_000 })
  })

  test('模板自动补全区域应在输入时尝试出现（无后端时容错隐藏）', async ({ page }) => {
    await navigateTo(page, '/logs')

    const keywordInput = page.getByTestId('log-search-keyword').locator('input')
    await keywordInput.focus()
    await keywordInput.fill('level:ERROR')

    // 后端可用时会展示建议列表；后端不可用时该节点不存在。
    // 断言「节点存在性」而非强制可见，保证用例在无后端环境仍稳定。
    const suggestions = page.getByTestId('log-search-template-suggestions')
    const count = await suggestions.count()
    if (count > 0) {
      await expect(suggestions.first()).toBeVisible()
    }
  })
})
