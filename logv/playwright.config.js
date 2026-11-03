// Playwright E2E 测试配置
//
// 说明：本仓库 Vite 开发服务器实际运行在 127.0.0.1:2022（见 vite.config.js）。
// 因此此处 webServer.url / use.baseURL 使用 2022，而非需求模板中的 5173，
// 以保证 E2E 用例能真正连接本地前端。如需切换端口，可用环境变量 E2E_BASE_URL 覆盖。
import { defineConfig, devices } from '@playwright/test'
import { existsSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { homedir } from 'node:os'

const BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:2022'

/**
 * 解析可用的 Chromium 可执行文件路径。
 *
 * 优先使用 Playwright 官方安装目录（ms-playwright）下任一 chromium-* 版本；
 * 这样在 `npx playwright install chromium` 因网络受限而失败时，仍可使用
 * 机器上已存在的浏览器构建运行 E2E，保证框架开箱即用。
 * 返回 null 表示交给 Playwright 自行解析（要求已安装匹配 revision）。
 */
function resolveChromiumExecutable() {
  const browsersRoot = join(homedir(), 'AppData', 'Local', 'ms-playwright')
  if (!existsSync(browsersRoot)) return null
  try {
    const dirs = readdirSync(browsersRoot)
      .filter((d) => d.startsWith('chromium-') && !d.includes('headless'))
      .sort()
      .reverse() // 取版本号最大的构建
    for (const dir of dirs) {
      const exe = join(browsersRoot, dir, 'chrome-win64', 'chrome.exe')
      if (existsSync(exe)) return exe
    }
  } catch {
    // 忽略读取失败，回退到默认解析。
  }
  return null
}

const chromiumExecutablePath = process.env.E2E_CHROMIUM_PATH || resolveChromiumExecutable()

export default defineConfig({
  testDir: './e2e',
  // 单测文件之间完全并行；每个 spec 内的用例按顺序执行以共享登录态。
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  // 失败时输出列表 + HTML 报告，便于定位。
  reporter: [['list'], ['html', { open: 'never' }]],
  // 全局超时：单个测试 30s，符合日志查询类交互的合理等待。
  timeout: 30_000,
  expect: {
    timeout: 10_000,
  },
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    // 失败时自动截图并保存到 test-results/，便于回溯。
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        // 若存在可用的本机 Chromium 则显式指定，避免依赖精确 revision 下载。
        ...(chromiumExecutablePath ? { executablePath: chromiumExecutablePath } : {}),
      },
    },
  ],
  webServer: {
    command: 'npm run dev',
    url: BASE_URL,
    reuseExistingServer: !process.env.CI,
    // 冷启动需要安装依赖 + Vite 初始化，给足就绪时间。
    timeout: 120_000,
    stdout: 'ignore',
    stderr: 'pipe',
  },
})
