# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: auth.spec.js >> 登录页面 >> 登录表单应支持输入用户名与密码
- Location: e2e\auth.spec.js:29:3

# Error details

```
TimeoutError: locator.fill: Timeout 15000ms exceeded.
Call log:
  - waiting for getByTestId('login-username').locator('input')

```

# Page snapshot

```yaml
- generic [ref=f1e9]:
  - generic [ref=f1e15]:
    - heading "Ydsz Trace" [level=1] [ref=f1e16]
    - paragraph [ref=f1e17]: 分布式日志追踪检索控制台
  - generic [ref=f1e18]:
    - generic [ref=f1e19]:
      - generic [ref=f1e20]: 用户名
      - textbox "用户名" [ref=f1e28]:
        - /placeholder: 请输入管理员用户名
    - generic [ref=f1e29]:
      - generic [ref=f1e30]: 密码
      - textbox "密码" [ref=f1e39]:
        - /placeholder: 请输入密码
    - button "登 录" [ref=f1e40] [cursor=pointer]
  - paragraph [ref=f1e43]: AI 驱动的日志排障定位平台 v2.0
```

# Test source

```ts
  1  | // 认证相关 E2E 用例：登录页加载、表单输入、未登录重定向。
  2  | //
  3  | // 运行方式：npx playwright test e2e/auth.spec.js
  4  | import { test, expect } from '@playwright/test'
  5  | import { login, waitForPageLoad, TEST_USER } from './utils/test-helpers'
  6  | 
  7  | test.describe('登录页面', () => {
  8  |   test.beforeEach(async ({ page }) => {
  9  |     // 清理 localStorage，确保从「未登录」状态开始。
  10 |     await page.goto('/login')
  11 |     await page.evaluate(() => window.localStorage.clear())
  12 |     await page.reload()
  13 |   })
  14 | 
  15 |   test('应正确加载登录页并展示品牌与表单', async ({ page }) => {
  16 |     await page.goto('/login')
  17 |     await waitForPageLoad(page)
  18 | 
  19 |     await expect(page.getByTestId('login-brand')).toBeVisible()
  20 |     await expect(page.getByTestId('login-brand-title')).toContainText('Ydsz Trace')
  21 |     await expect(page.getByTestId('login-brand-subtitle')).toBeVisible()
  22 | 
  23 |     await expect(page.getByTestId('login-form')).toBeVisible()
  24 |     await expect(page.getByTestId('login-username')).toBeVisible()
  25 |     await expect(page.getByTestId('login-password')).toBeVisible()
  26 |     await expect(page.getByTestId('login-submit')).toBeVisible()
  27 |   })
  28 | 
  29 |   test('登录表单应支持输入用户名与密码', async ({ page }) => {
  30 |     const usernameInput = page.getByTestId('login-username').locator('input')
  31 |     const passwordInput = page.getByTestId('login-password').locator('input')
  32 | 
> 33 |     await usernameInput.fill(TEST_USER)
     |                         ^ TimeoutError: locator.fill: Timeout 15000ms exceeded.
  34 |     await passwordInput.fill('secret-password')
  35 | 
  36 |     await expect(usernameInput).toHaveValue(TEST_USER)
  37 |     await expect(passwordInput).toHaveValue('secret-password')
  38 |   })
  39 | 
  40 |   test('未登录访问受保护页面应重定向到 /login', async ({ page }) => {
  41 |     // 直接访问受保护路由，无任何登录态。
  42 |     await page.goto('/dashboard')
  43 |     await waitForPageLoad(page)
  44 | 
  45 |     // AuthGuard 会重定向到 /login（并带 redirect query）。
  46 |     await expect(page).toHaveURL(/\/login/)
  47 |   })
  48 | })
  49 | 
  50 | test.describe('登录后跳转', () => {
  51 |   test('注入登录态后访问首页不应再停留在登录页', async ({ page }) => {
  52 |     await login(page, TEST_USER, 'any-password')
  53 |     await expect(page).not.toHaveURL(/\/login/)
  54 |   })
  55 | })
  56 | 
```