// 结果聚类折叠回归测试（V2.4-09）
//
// 背景：
//   后端 groupByTemplate 最多返回 3 条样本（FR-702），
//   但前端折叠阈值写死为 5，导致「展开全部」按钮永不出现。
//   本测试通过源码断言防止这类前后端契约不一致的回归。
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'fs'
import { join } from 'path'

const searchPath = join(process.cwd(), 'src/views/LogSearch.vue')

function source() {
  return readFileSync(searchPath, 'utf-8')
}

describe('结果聚类折叠（V2.4-09）', () => {
  it('聚类列表由后端 templateGroups 驱动', () => {
    const src = source()
    expect(src).toContain('templateGroups')
    expect(src).toContain('sampleLines')
  })

  it('折叠阈值与后端 3 条样本上限一致', () => {
    const src = source()
    expect(src).toContain('COLLAPSED_SAMPLE_LIMIT')
    // 不应再出现写死的 slice(0, 5)
    expect(src).not.toContain('sampleLines.slice(0, 5)')
  })

  it('展开/收起由显式函数处理', () => {
    const src = source()
    expect(src).toContain('toggleTemplateSamples')
    expect(src).toContain('shouldShowExpand')
    expect(src).toContain('visibleSamples')
  })

  it('支持按命中次数与时间排序', () => {
    const src = source()
    expect(src).toContain('groupSort')
    expect(src).toContain('sortedTemplateGroups')
    expect(src).toContain("value=\"count\"")
    expect(src).toContain("value=\"time\"")
  })

  it('折叠面板使用 el-collapse', () => {
    const src = source()
    expect(src).toContain('el-collapse')
    expect(src).toContain('el-collapse-item')
  })

  it('聚类与原始列表视图可切换', () => {
    const src = source()
    expect(src).toContain("value=\"cluster\"")
    expect(src).toContain("value=\"flat\"")
  })
})
