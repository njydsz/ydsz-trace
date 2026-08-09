import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'

// ESLint 9 flat config — Vue 3 + Prettier 协作
// prettier 负责格式化，eslint 负责逻辑质量，避免规则冲突。
export default [
  {
    // 全局忽略
    ignores: ['dist/**', 'coverage/**', 'node_modules/**', '*.min.js', 'public/**'],
  },
  // 基础 JS 推荐规则
  js.configs.recommended,
  // Vue 插件：essential 级别起步（后续可升级到 recommended/strongly-recommended）
  ...pluginVue.configs['flat/essential'],
  {
    files: ['**/*.{js,jsx,mjs,cjs,vue}'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        // 浏览器全局
        window: 'readonly',
        document: 'readonly',
        console: 'readonly',
        navigator: 'readonly',
        location: 'readonly',
        fetch: 'readonly',
        localStorage: 'readonly',
        sessionStorage: 'readonly',
        setTimeout: 'readonly',
        clearTimeout: 'readonly',
        setInterval: 'readonly',
        clearInterval: 'readonly',
        URL: 'readonly',
        URLSearchParams: 'readonly',
        FileReader: 'readonly',
        Blob: 'readonly',
        AbortController: 'readonly',
        ReadableStream: 'readonly',
        EventSource: 'readonly',
        HTMLElement: 'readonly',
        CustomEvent: 'readonly',
        // Vitest 全局
        describe: 'readonly',
        it: 'readonly',
        test: 'readonly',
        expect: 'readonly',
        vi: 'readonly',
        beforeEach: 'readonly',
        afterEach: 'readonly',
        beforeAll: 'readonly',
        afterAll: 'readonly',
      },
    },
    rules: {
      // === 代码质量 ===
      'no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      'prefer-const': 'warn',
      'no-var': 'error',
      eqeqeq: ['warn', 'always'],

      // === Vue 推荐 ===
      'vue/multi-word-component-names': 'off',
      'vue/no-v-html': 'warn',
      'vue/component-name-in-template-casing': ['warn', 'PascalCase'],
      'vue/component-definition-name-casing': ['warn', 'PascalCase'],
      'vue/define-macros-order': 'warn',
      'vue/no-mutating-props': 'error',
      'vue/no-unused-refs': 'warn',
      'vue/require-v-for-key': 'error',

      // === 与 Prettier 协作 ===
      'vue/html-self-closing': 'off',  // prettier 处理
      'vue/singleline-html-element-content-newline': 'off',  // prettier 处理
    },
  },
  // 测试文件放宽部分规则
  {
    files: ['**/*.spec.js', '**/*.test.js', '**/tests/**'],
    rules: {
      'no-console': 'off',
      'no-unused-vars': 'off',
    },
  },
]
