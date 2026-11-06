// Live Tail 虚拟滚动回归测试（V2.4-08）
//
// 背景：
//   1. V2.1 时代 LogTail 用 500/1000 行环形缓冲硬截断，长时追踪丢历史。
//   2. V2.4-08 引入虚拟滚动后，原实现用 RecycleScroller + 固定 item-size=28，
//      但日志开启 pre-wrap 后行高可变，固定尺寸会导致滚动错乱与白屏。
//
// 本测试通过源码断言防止这两类回归。
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'fs'
import { join } from 'path'

const tailPath = join(process.cwd(), 'src/views/LogTail.vue')

function source() {
  return readFileSync(tailPath, 'utf-8')
}

describe('Live Tail 虚拟滚动（V2.4-08）', () => {
  it('使用 DynamicScroller 而非固定高度的 RecycleScroller', () => {
    const src = source()
    expect(src).toContain('DynamicScroller')
    expect(src).toContain('DynamicScrollerItem')
  })

  it('不再使用固定 item-size（改用 min-item-size）', () => {
    const src = source()
    expect(src).not.toContain(':item-size=')
    expect(src).toContain(':min-item-size=')
  })

  it('行高不再被固定，支持换行', () => {
    const src = source()
    // 旧写法固定 height: 28px 与 pre-wrap 冲突（注意排除 min-height）
    expect(src).not.toMatch(/\.log-line\s*\{[^}]*(?<!min-)height:\s*28px/)
    expect(src).toContain('min-height: 28px')
    expect(src).toContain('pre-wrap')
  })

  it('行高变化时通知 scroller 重新测量', () => {
    const src = source()
    // size-dependencies 是 DynamicScrollerItem 重算高度的触发条件
    expect(src).toContain(':size-dependencies=')
  })

  it('缓冲区上限已提升（不再是 500/1000 硬截断）', () => {
    const src = source()
    expect(src).toMatch(/RING_BUFFER_SIZE\s*=\s*\d+/)
    const m = src.match(/RING_BUFFER_SIZE\s*=\s*(\d+)/)
    expect(Number(m[1])).toBeGreaterThanOrEqual(2000)
  })

  it('保留按级别高亮能力', () => {
    const src = source()
    expect(src).toContain('getLineClass')
  })
})
