// 认证相关 E2E 用例：登录页加载、表单输入、未登录重定向。
//
// 运行方式：npx playwright test e2e/auth.spec.js
import { test, expect } from '@playwright/test'
import { login, waitForPageLoad, TEST_USER } from './utils/test-helpers'

test.describe('登录页面', () => {
  test.beforeEach(async ({ page }) => {
    // 清理 localStorage，确保从「未登录」状态开始。
    await page.goto('/login')
    await page.evaluate(() => window.localStorage.clear())
    await page.reload()
  })

  test('应正确加载登录页并展示品牌与表单', async ({ page }) => {
    await page.goto('/login')
    await waitForPageLoad(page)

    await expect(page.getByTestId('login-brand')).toBeVisible()
    await expect(page.getByTestId('login-brand-title')).toContainText('Ydsz Trace')
    await expect(page.getByTestId('login-brand-subtitle')).toBeVisible()

    await expect(page.getByTestId('login-form')).toBeVisible()
    await expect(page.getByTestId('login-username')).toBeVisible()
    await expect(page.getByTestId('login-password')).toBeVisible()
    await expect(page.getByTestId('login-submit')).toBeVisible()
  })

  test('登录表单应支持输入用户名与密码', async ({ page }) => {
    const usernameInput = page.getByTestId('login-username').locator('input')
    const passwordInput = page.getByTestId('login-password').locator('input')

    await usernameInput.fill(TEST_USER)
    await passwordInput.fill('secret-password')

    await expect(usernameInput).toHaveValue(TEST_USER)
    await expect(passwordInput).toHaveValue('secret-password')
  })

  test('未登录访问受保护页面应重定向到 /login', async ({ page }) => {
    // 直接访问受保护路由，无任何登录态。
    await page.goto('/dashboard')
    await waitForPageLoad(page)

    // AuthGuard 会重定向到 /login（并带 redirect query）。
    await expect(page).toHaveURL(/\/login/)
  })
})

test.describe('登录后跳转', () => {
  test('注入登录态后访问首页不应再停留在登录页', async ({ page }) => {
    await login(page, TEST_USER, 'any-password')
    await expect(page).not.toHaveURL(/\/login/)
  })
})
