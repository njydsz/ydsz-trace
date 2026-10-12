// 安全 SSE 连接封装（fetch + ReadableStream 方案）。
//
// EventSource 原生对象不支持自定义 header，无法携带 CSRF 防护头，
// 因此使用 fetch + getReader() 手动解析 SSE 流。
//
// 特性：
//   - 请求头附加 X-Requested-With: XMLHttpRequest（CSRF 防护）
//   - 401 响应时清空登录态并跳转 /login
//   - 指数退避自动重连（可配置重试次数与间隔）
//   - 提供 close() / isActive() 生命周期接口
import { useAuth } from '@/store/auth'

/**
 * 解析 SSE event block 为 { event, data } 对象。
 * 符合 W3C EventSource 格式：
 *   event: message
 *   data: hello world
 * @param {string} block - 以 \n\n 分隔的原始 SSE 块
 * @returns {{ event: string, data: string } | null}
 */
export function parseSSEEvent(block) {
  let event = 'message'
  let data = ''
  for (const line of block.split('\n')) {
    if (line.startsWith('event: ')) {
      event = line.slice(7).trim()
    } else if (line.startsWith('data: ')) {
      data += (data ? '\n' : '') + line.slice(6)
    }
  }
  if (!data) return null
  return { event, data }
}

/**
 * 创建安全的 SSE 连接。
 * 附加 X-Requested-With 头用于 CSRF 防护，
 * 在接收到 401 时清空登录态并跳转登录页。
 *
 * @param {string} url - SSE 端点 URL
 * @param {object} [options] - 可选配置
 * @param {function} [options.onMessage] - 收到 SSE 事件时的回调 (event, data) => void
 * @param {function} [options.onError] - 错误回调 (error) => void
 * @param {function} [options.onClose] - 连接关闭回调 () => void
 * @param {number} [options.maxRetries=3] - 最大重连次数
 * @param {number} [options.retryInterval=3000] - 基础重连间隔毫秒（指数退避基数）
 * @param {string} [options.method='GET'] - HTTP 请求方法
 * @param {object|string} [options.body] - POST 请求体
 * @returns {{ close: Function, isActive: () => boolean }}
 *
 * @example
 * const conn = createSSE('/logs/tail', {
 *   method: 'POST',
 *   body: JSON.stringify({ item: 1, date: '20250101' }),
 *   onMessage: (evt, data) => console.log(evt, data),
 * })
 * conn.isActive()  // true
 * conn.close()     // 关闭连接
 */
export function createSSE(url, options = {}) {
  const {
    onMessage,
    onError,
    onClose,
    maxRetries = 3,
    retryInterval = 3000,
    method = 'GET',
    body,
  } = options

  let active = false
  let reader = null
  let abortCtrl = null
  let retryCount = 0

  function connect() {
    if (!active) return

    abortCtrl = new AbortController()

    const fetchOptions = {
      method,
      headers: {
        'X-Requested-With': 'XMLHttpRequest',
        'Accept': 'text/event-stream',
      },
      credentials: 'include',
      signal: abortCtrl.signal,
    }

    if (body && method !== 'GET') {
      fetchOptions.headers['Content-Type'] = 'application/json'
      fetchOptions.body = typeof body === 'string' ? body : JSON.stringify(body)
    }

    fetch(url, fetchOptions).then(response => {
      if (response.status === 401) {
        active = false
        // 清空登录态并跳转登录页
        try {
          useAuth().clear()
        } catch {
          // store 可能未初始化，忽略
        }
        if (window.location.pathname !== '/login') {
          window.location.href = '/login'
        }
        return
      }

      if (!response.ok) {
        const err = new Error(`SSE 连接失败: HTTP ${response.status}`)
        if (retryCount < maxRetries) {
          scheduleRetry()
        } else if (onError) {
          onError(err)
        }
        return
      }

      // 重置重试计数（连接成功后）
      retryCount = 0

      reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      function readChunk() {
        if (!active) return
        reader.read().then(({ done, value }) => {
          if (done) {
            if (active) {
              // 服务端主动关闭，尝试重连
              if (retryCount < maxRetries) {
                scheduleRetry()
              } else {
                active = false
                if (onClose) onClose()
              }
            }
            return
          }
          buffer += decoder.decode(value, { stream: true })
          const blocks = buffer.split('\n\n')
          buffer = blocks.pop()
          for (const block of blocks) {
            const event = parseSSEEvent(block)
            if (event && onMessage) onMessage(event.event, event.data)
          }
          readChunk()
        }).catch(err => {
          if (err.name === 'AbortError') return // 主动关闭
          if (!active) return
          if (retryCount < maxRetries) {
            retryCount++
            scheduleRetry()
          } else {
            active = false
            if (onError) onError(err)
            if (onClose) onClose()
          }
        })
      }
      readChunk()
    }).catch(err => {
      if (err.name === 'AbortError') return
      if (!active) return
      if (retryCount < maxRetries) {
        scheduleRetry()
      } else {
        active = false
        if (onError) onError(err)
        if (onClose) onClose()
      }
    })
  }

  function scheduleRetry() {
    retryCount++
    const delay = retryInterval * Math.pow(2, retryCount - 1)
    setTimeout(connect, delay)
  }

  function close() {
    active = false
    if (reader) {
      reader.cancel().catch(() => {})
      reader = null
    }
    if (abortCtrl) {
      abortCtrl.abort()
      abortCtrl = null
    }
    if (onClose) onClose()
  }

  function isActive() {
    return active
  }

  // 启动连接
  active = true
  connect()

  return { close, isActive }
}
