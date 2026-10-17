// 侧边抽屉的键盘与焦点行为：Esc 关闭、打开时焦点收进抽屉、关闭后还给触发钮。
// 各面板的抽屉结构相同，收成一个组合式函数才不会漏掉焦点陷阱。
import { onBeforeUnmount, ref, watch, type Ref } from 'vue'

/**
 * @param open 控制抽屉显隐的 ref（调用方持有；真假值皆可，用空串表示关）
 * @param onClose 关闭回调（由调用方把 open 置空）
 * @returns drawerRef 绑到 <section class="drawer-side"> 上
 */
export function useDrawer<T extends boolean | string>(open: Ref<T>, onClose: () => void) {
  const drawerRef = ref<HTMLElement | null>(null)
  // 打开前的焦点：关闭时还回去，键盘用户不会一睁眼就迷失在页面头部
  let returnFocus: HTMLElement | null = null

  // 抽屉内可聚焦元素：可见 + 未禁用 + 非隐藏。顺序即 Tab 序。
  function focusables(root: HTMLElement): HTMLElement[] {
    const sel = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
    return Array.from(root.querySelectorAll<HTMLElement>(sel)).filter(
      (el) => el.offsetParent !== null || el.getClientRects().length > 0,
    )
  }

  function trap(e: KeyboardEvent) {
    if (!open.value) return
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
      return
    }
    if (e.key !== 'Tab') return
    const root = drawerRef.value
    if (!root) return
    const items = focusables(root)
    if (!items.length) {
      // 抽屉里没有可聚焦元素：Tab 不能穿透到背后
      e.preventDefault()
      return
    }
    const first = items[0]
    const last = items[items.length - 1]
    const active = document.activeElement as HTMLElement | null
    // 焦点在抽屉外（或丢了）：Tab 收回到第一个
    if (!active || !root.contains(active)) {
      e.preventDefault()
      first.focus()
      return
    }
    if (e.shiftKey && active === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && active === last) {
      e.preventDefault()
      first.focus()
    }
  }

  watch(open, (on) => {
    if (on) {
      returnFocus = document.activeElement as HTMLElement | null
      // 下一帧再聚焦：抽屉刚挂载，直接 focus 会因元素还没布局而失败
      requestAnimationFrame(() => {
        const root = drawerRef.value
        if (!root) return
        const items = focusables(root)
        // 默认聚焦第一个输入框而不是关闭钮：用户开抽屉就是要填东西
        const target = items.find((el) => el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') || items[0]
        target?.focus()
      })
      return
    }
    // 关闭后把焦点还给触发钮，且只在该元素还在文档里时
    if (returnFocus && document.contains(returnFocus)) {
      returnFocus.focus()
    }
    returnFocus = null
  })

  document.addEventListener('keydown', trap, true)
  onBeforeUnmount(() => document.removeEventListener('keydown', trap, true))

  return { drawerRef }
}
