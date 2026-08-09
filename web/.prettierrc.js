// Prettier 配置 — 与 ESLint 分工：Prettier 管格式化，ESLint 管质量。
export default {
  semi: false,
  singleQuote: true,
  trailingComma: 'all',
  printWidth: 100,
  tabWidth: 2,
  useTabs: false,
  arrowParens: 'always',
  endOfLine: 'lf',
  bracketSpacing: true,
  vueIndentScriptAndStyle: false,
  // 单行模板属性较多时保持可读
  singleAttributePerLine: false,
}
