/**
 * 全局快捷键 composable：注册 / 注销 / 清空任意 key handler；首次注册时挂全局 keydown，全部注销时移除。
 * 输入框（input / textarea / contentEditable）中除 ESC 外不触发。
 */

export type ShortcutHandler = (e: KeyboardEvent) => void

export interface ShortcutsApi {
  registerShortcut: (key: string, handler: ShortcutHandler) => void
  unregisterShortcut: (key: string) => void
  clearAll: () => void
}

const shortcuts = new Map<string, ShortcutHandler>()

function normalizeKey(e: KeyboardEvent): string {
  const parts: string[] = []
  if (e.ctrlKey || e.metaKey) parts.push('ctrl')
  if (e.shiftKey) parts.push('shift')
  if (e.altKey) parts.push('alt')
  parts.push(e.key.toLowerCase())
  return parts.join('+')
}

function onKeyDown(e: KeyboardEvent): void {
  // 在输入框中不触发全局快捷键（除了 ESC）
  const target = e.target as HTMLElement
  const isInput =
    target.tagName === 'INPUT' ||
    target.tagName === 'TEXTAREA' ||
    target.isContentEditable

  const key = normalizeKey(e)

  // ESC 始终可用
  if (key === 'escape') {
    const handler = shortcuts.get('escape')
    if (handler) {
      e.preventDefault()
      handler(e)
    }
    return
  }

  // 其他快捷键在输入框中不触发
  if (isInput) return

  const handler = shortcuts.get(key)
  if (handler) {
    e.preventDefault()
    handler(e)
  }
}

let listenerCount = 0

/** 全局快捷键 composable。 */
export function useShortcuts(): ShortcutsApi {
  function registerShortcut(key: string, handler: ShortcutHandler): void {
    const normalizedKey = key.toLowerCase().replace(/\s/g, '')
    shortcuts.set(normalizedKey, handler)

    // 第一个快捷键时注册全局监听
    if (listenerCount === 0) {
      document.addEventListener('keydown', onKeyDown)
    }
    listenerCount++
  }

  function unregisterShortcut(key: string): void {
    const normalizedKey = key.toLowerCase().replace(/\s/g, '')
    shortcuts.delete(normalizedKey)

    listenerCount--
    // 没有快捷键时移除全局监听
    if (listenerCount <= 0) {
      listenerCount = 0
      document.removeEventListener('keydown', onKeyDown)
    }
  }

  function clearAll(): void {
    shortcuts.clear()
    listenerCount = 0
    document.removeEventListener('keydown', onKeyDown)
  }

  return {
    registerShortcut,
    unregisterShortcut,
    clearAll
  }
}
