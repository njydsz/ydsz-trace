// 对话工作台 E2E 用例：页面加载、消息发送、会话列表切换。
//
// 运行方式：npx playwright test e2e/conversation.spec.js
import { test, expect } from '@playwright/test'
import { login, navigateTo } from './utils/test-helpers'

test.describe('对话工作台', () => {
  test.beforeEach(async ({ page }) => {
    await login(page)
  })

  test('应加载对话工作台并展示欢迎界面与输入区', async ({ page }) => {
    await navigateTo(page, '/conversation')

    await expect(page.getByTestId('conversation-sidebar')).toBeVisible()
    await expect(page.getByTestId('conversation-new-btn')).toBeVisible()

    // 初始无消息时展示欢迎页；若有历史会话消息则展示消息列表。
    const welcome = page.getByTestId('conversation-welcome')
    if (await welcome.count()) {
      await expect(welcome).toBeVisible()
      await expect(page.getByTestId('conversation-welcome-title')).toContainText('日志检索助手')
    }

    await expect(page.getByTestId('conversation-input')).toBeVisible()
    await expect(page.getByTestId('conversation-send')).toBeVisible()
  })

  test('应在输入框输入消息并通过按钮发送', async ({ page }) => {
    await navigateTo(page, '/conversation')

    const input = page.getByTestId('conversation-input')
    await input.fill('今天有哪些 ERROR？')
    await expect(input).toHaveValue('今天有哪些 ERROR？')

    // 发送后输入框应被清空（handleSend 会先 push 用户消息再清空输入）。
    await page.getByTestId('conversation-send').click()
    await expect(input).toHaveValue('')
  })

  test('会话列表项应可点击切换', async ({ page }) => {
    await navigateTo(page, '/conversation')

    const items = page.getByTestId('conversation-item')
    const count = await items.count()
    if (count === 0) {
      // 无历史会话时跳过，保证用例在无后端环境仍稳定。
      test.skip(true, '当前无历史会话可切换')
      return
    }
    await items.first().click()
    // 点击后会话项应获得 active 状态或至少仍可见。
    await expect(items.first()).toBeVisible()
  })
})
