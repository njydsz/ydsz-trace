import http from './http'

// NL2DSL 自然语言转结构化检索参数 API—与后端 /logs/nl2dsl/parse 对齐。

// nl2dslParse 将自然语言查询解析为结构化的日志检索 DSL 参数。
// @param {string} query - 用户自然语言输入
// @returns {Promise<object>} 解析结果 { level, date, startTime, endTime, key, timeRangeDetail, dslConditions, intentType }
export function nl2dslParse(query) {
  return http.post('/logs/nl2dsl/parse', { query })
}
