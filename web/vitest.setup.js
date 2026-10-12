import { config } from '@vue/test-utils'

// 注册 Element Plus v-loading 指令的 noop 实现，避免测试中 "Failed to resolve directive: loading" 警告
config.global.directives = {
  loading: () => {
    // noop in tests — v-loading 指令不做任何事情，避免测试中报错
  },
  // 防止其他 EP 指令报错
  permission: () => {},
}
