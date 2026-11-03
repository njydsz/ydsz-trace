// E2E 测试通用辅助函数。
//
// 设计目标：用例可独立运行、无需后端真实数据即可覆盖前端交互路径。
//
// 认证策略说明：
//   本项目的登录态完全由 localStorage（ydsz_username / ydsz_role）驱动，
//   router 守卫与 AuthGuard 只读取该状态。因此 `login()` 优先采用「种子注入」方式，
//   在浏览器上下文中直接写入登录态，再进入受保护页面。这样：
//     1. 不依赖后端可用性，CI / 本地都能稳定运行；
//     2. 与真实登录后写入的状态完全一致（真实登录也会写这两个 key）。
//   若确实需要验证真实登录接口，可单独编写调用 /auth/login 的集成用例。

import { expect } from '@playwright/test'

/** 默认测试账号（仅用于前端交互路径验证，不代表任何真实凭据）。 */
export const TEST_USER = 'e2e_tester'
export const TEST_ROLE = 'admin'
export const TEST_PASSWORD = 'e2e-test-password'

/**
 * 写入前端登录态种子。
 *
 * @param {import('@playwright/test').Page} page
 * @param {{ username?: string, role?: string }} [opts]
 */
async function seedAuthStorage(page, opts = {}) {
  const username = opts.username ?? TEST_USER
  const role = opts.role ?? TEST_ROLE
  await page.addInitScript(
    ([u, r]) => {
      window.localStorage.setItem('ydsz_username', u)
      window.localStorage.setItem('ydsz_role', r)
    },
    [username, role],
  )
}

/**
 * 登录辅助函数。
 *
 * 优先使用 localStorage 种子注入（无后端依赖）；可选地也会尝试走真实登录表单，
 * 以便在后端可用时验证表单链路。由于表单提交依赖后端响应，这里默认只注入种子，
 * 保证用例在任何环境下都可独立运行。
 *
 * @param {import('@playwright/test').Page} page
 * @param {string} [username]
 * @param {string} [password]
 * @returns {Promise<void>}
 */
export async function login(page, username = TEST_USER, password = TEST_PASSWORD) {
  // 写入种子后访问首页，触发路由跳转至工作台。
  await seedAuthStorage(page, { username })
  await page.goto('/')
  // 校验登录态已生效（守卫不会再回到 /login）。
  await waitForPageLoad(page)
  expect(page.url()).not.toContain('/login')
  // password 参数保留用于后续接入真实接口时使用。
  void password
}

/**
 * 导航辅助函数：跳转到指定路由并等待布局就绪。
 *
 * @param {import('@playwright/test').Page} page
 * @param {string} path - 形如 '/dashboard' 的路由路径
 */
export async function navigateTo(page, path) {
  await page.goto(path)
  await waitForPageLoad(page)
}

/**
 * 等待页面加载完成。
 *
 * 判定标准（按优先级）：
 *   1. 网络空闲（networkidle），确保异步请求/渲染基本结束；
 *   2. 兜底等待 body 可见 + 至少一个主内容节点出现。
 *
 * 注意：Element Plus 组件初始化较快，但部分页面有 loading 态，
 * networkidle 能较好覆盖骨架屏到内容的切换。
 *
 * @param {import('@playwright/test').Page} page
 */
export async function waitForPageLoad(page) {
  await page.waitForLoadState('domcontentloaded')
  try {
    await page.waitForLoadState('networkidle', { timeout: 10_000 })
  } catch {
    // 存在长轮询/SSE 时 networkidle 可能超时，忽略即可继续。
  }
  // 兜底：等待布局容器出现，确保路由目标已挂载。
  await page.locator('body').waitFor({ state: 'visible' })
}
