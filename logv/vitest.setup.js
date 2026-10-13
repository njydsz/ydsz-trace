import { config } from '@vue/test-utils'

// 注册 Element Plus v-loading 指令的 noop 实现，避免测试中 "Failed to resolve directive: loading" 警告
config.global.directives = {
  loading: () => {
    // noop in tests — v-loading 指令不做任何事情，避免测试中报错
  },
  // 防止其他 EP 指令报错
  permission: () => {},
}

// 注册常用 Element Plus 组件 stubs，避免 "Failed to resolve component" 警告
config.global.stubs = {
  // eui-icon 系列的图标组件
  'el-icon': { template: '<span><slot /></span>' },
  'Share': { template: '<span><slot /></span>' },
  'Promotion': { template: '<span><slot /></span>' },
  'DocumentCopy': { template: '<span><slot /></span>' },
  'Platform': { template: '<span><slot /></span>' },
  'Sunny': { template: '<span><slot /></span>' },
  'Moon': { template: '<span><slot /></span>' },
  // El 表单控件
  'el-input': { template: '<div><input /></div>' },
  'el-divider': { template: '<hr />' },
  'el-card': { template: '<div><slot /></div>' },
  'el-collapse': { template: '<div><slot /></div>' },
  'el-collapse-item': { template: '<div><slot /></div>' },
  'el-radio-group': { template: '<div><slot /></div>' },
  'el-radio-button': { template: '<button />' },
  'el-tag': { template: '<span><slot /></span>' },
  'el-alert': { template: '<div><slot /></div>' },
  'el-empty': { template: '<div><slot /></div>' },
  'el-dialog': { template: '<div><slot /></div>' },
  'el-switch': { template: '<div />' },
}
