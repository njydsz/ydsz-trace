/**
 * useKeyboardShortcuts — 全局键盘快捷键 composable
 *
 * 提供统一的事件注册、取消注册与动态映射接口。
 * - 自动防止输入框/可编辑元素内触发
 * - onMounted 时 attach、onUnmounted 时 detach
 * - 开放 configure(map) 接口动态追加快捷键
 *
 * 快捷键映射格式：
 *   { key: 'k', ctrl: true, handler: fn, preventDefault: true }
 */
import { ref, onMounted, onUnmounted } from 'vue'

// 默认快捷键映射表
const DEFAULT_SHORTCUTS = [
  {
    id: 'open-search',
    key: 'k',
    ctrl: true,
    meta: true,
    description: '唤起搜索对话框',
    handler: () => {
      document.dispatchEvent(new CustomEvent('shortcut:open-search'))
    },
  },
  {
    id: 'toggle-help',
    key: '/',
    ctrl: true,
    meta: true,
    description: '切换帮助面板',
    handler: () => {
      document.dispatchEvent(new CustomEvent('shortcut:toggle-help'))
    },
  },
  {
    id: 'close-modal',
    key: 'Escape',
    ctrl: false,
    meta: false,
    description: '关闭最前弹窗',
    handler: () => {
      document.dispatchEvent(new CustomEvent('shortcut:close-modal'))
    },
  },
  {
    id: 'list-down',
    key: 'j',
    ctrl: false,
    meta: false,
    description: '列表焦点下移',
    context: 'list',
    handler: () => {
      document.dispatchEvent(new CustomEvent('shortcut:list-down'))
    },
  },
  {
    id: 'list-up',
    key: 'k',
    ctrl: false,
    meta: false,
    description: '列表焦点上移',
    context: 'list',
    handler: () => {
      document.dispatchEvent(new CustomEvent('shortcut:list-up'))
    },
  },
]

// 检查事件目标是否为表单输入元素
function isFormInput(target) {
  if (!target || !target.tagName) return false
  const tag = target.tagName.toLowerCase()
  return tag === 'input' || tag === 'textarea' || tag === 'select' || target.isContentEditable
}

// 检查快捷键是否匹配
function matches(e, shortcut) {
  // key 匹配（不区分大小写）
  if (e.key.toLowerCase() !== shortcut.key.toLowerCase()) return false

  // Ctrl/Cmd 要求：
  //   - 若 shortcut.ctrl 和 shortcut.meta 均为 true，则 Ctrl 或 Cmd 任一按下即匹配
  //   - 若仅 shortcut.ctrl 为 true，则仅 Ctrl 按下时匹配
  //   - 若仅 shortcut.meta 为 true，则仅 Cmd 按下时匹配
  //   - 若均为 false，则要求无修饰键
  if (shortcut.ctrl && shortcut.meta) {
    if (!e.ctrlKey && !e.metaKey) return false
  } else if (shortcut.ctrl) {
    if (!e.ctrlKey) return false
  } else if (shortcut.meta) {
    if (!e.metaKey) return false
  } else {
    // 非修饰键快捷键：当按下 Ctrl/Cmd 时不匹配（防止与 Ctrl+K 等冲突）
    if (e.ctrlKey || e.metaKey) return false
  }

  return true
}

/**
 * @param {Object} options
 * @param {boolean} options.bindListeners - 是否在 onMounted/onUnmounted 时自动绑定/解绑（默认 true）
 * @returns {{ shortcuts: Ref<Array>, configure: Function, attach: Function, detach: Function }}
 */
export function useKeyboardShortcuts(options = {}) {
  const { bindListeners = true } = options
  const shortcuts = ref([...DEFAULT_SHORTCUTS])
  let _handler = null

  function onKeyDown(e) {
    // 表单输入中不触发
    if (isFormInput(e.target)) return

    for (const shortcut of shortcuts.value) {
      if (!matches(e, shortcut)) continue
      if (shortcut.preventDefault !== false) {
        e.preventDefault()
      }
      shortcut.handler(e)
      break
    }
  }

  function attach() {
    if (_handler) return
    _handler = onKeyDown
    document.addEventListener('keydown', _handler)
  }

  function detach() {
    if (!_handler) return
    document.removeEventListener('keydown', _handler)
    _handler = null
  }

  /**
   * 动态注册额外快捷键
   * @param {Array<Object>} map - 格式同 DEFAULT_SHORTCUTS
   */
  function configure(map) {
    if (!Array.isArray(map)) return
    for (const item of map) {
      if (!item || !item.key || typeof item.handler !== 'function') continue
      const id = item.id || `custom-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
      const existingIdx = shortcuts.value.findIndex(s => s.id === id)
      const entry = {
        id,
        key: item.key,
        ctrl: !!item.ctrl,
        meta: !!item.meta,
        preventDefault: item.preventDefault !== false,
        description: item.description || '',
        context: item.context || null,
        handler: item.handler,
      }
      if (existingIdx >= 0) {
        shortcuts.value[existingIdx] = entry
      } else {
        shortcuts.value.push(entry)
      }
    }
  }

  if (bindListeners) {
    onMounted(attach)
    onUnmounted(detach)
  }

  return { shortcuts, configure, attach, detach }
}
