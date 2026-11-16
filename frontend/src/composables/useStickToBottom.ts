// 贴底跟随：内容在长（流式）时把视口钉在最新消息上，用户一滚开就放行，回到底部再接上。
// 布局、content-visibility 与程序化 scrollTop 都会触发与手势一模一样的 scroll 事件，
// 那些不能清掉跟随标记——只有滚轮 / 触摸 / 拖滚动条 / 键盘翻页这些显式手势才算「用户走开了」。
import { ref, type Ref } from 'vue'

/** 距底多少像素内算「贴底」。 */
const THRESHOLD = 24

export function useStickToBottom(scroller: Ref<HTMLElement | null>, content: Ref<HTMLElement | null>) {
  const hasOverflow = ref(false)
  const atTop = ref(true)
  const atBottom = ref(true)

  let stickToBottom = true
  let programmatic = false
  let programmaticGeneration = 0
  let syncFrame = 0
  let resizeObserver: ResizeObserver | null = null
  let touchY = 0
  const guardFrames = new Set<number>()
  let removeScrollend: (() => void) | null = null
  let scrollendTimer: number | null = null

  function updateState(): void {
    const el = scroller.value
    if (!el) return
    const maxScrollTop = Math.max(0, el.scrollHeight - el.clientHeight)
    hasOverflow.value = maxScrollTop > 1
    atTop.value = el.scrollTop <= THRESHOLD
    atBottom.value = maxScrollTop - el.scrollTop <= THRESHOLD
  }

  function clearScrollendTimer(): void {
    removeScrollend?.()
    removeScrollend = null
    if (scrollendTimer === null) return
    window.clearTimeout(scrollendTimer)
    scrollendTimer = null
  }

  function afterFrame(callback: () => void): void {
    const frame = requestAnimationFrame(() => {
      guardFrames.delete(frame)
      callback()
    })
    guardFrames.add(frame)
  }

  // 程序化滚动的守卫期：晚到的 scroll 事件落在守卫里不算手势。
  function beginProgrammatic(): void {
    programmatic = true
    const generation = ++programmaticGeneration
    clearScrollendTimer()
    afterFrame(() => {
      afterFrame(() => {
        if (generation !== programmaticGeneration) return
        programmatic = false
      })
    })
  }

  function endProgrammatic(): void {
    programmaticGeneration += 1
    for (const frame of guardFrames) cancelAnimationFrame(frame)
    guardFrames.clear()
    programmatic = false
    clearScrollendTimer()
  }

  function pinToBottom(): void {
    const el = scroller.value
    if (!el || !stickToBottom) return
    beginProgrammatic()
    el.scrollTop = el.scrollHeight
  }

  // 内容长高后补一次钉底：late markdown 布局可能在首钉之后才把高度撑开。
  function syncAfterContentResize(): void {
    pinToBottom()
    updateState()
    const el = scroller.value
    if (!el || !stickToBottom) return
    if (el.scrollHeight - el.clientHeight - el.scrollTop > THRESHOLD) {
      pinToBottom()
      updateState()
    }
  }

  function scheduleSync(): void {
    if (syncFrame) return
    syncFrame = requestAnimationFrame(() => {
      syncFrame = 0
      syncAfterContentResize()
    })
  }

  function onScroll(): void {
    updateState()
    if (programmatic) return
    // 晚到的布局 scroll 可能落在守卫期之后：只有显式手势才放行跟随。
    if (atBottom.value) stickToBottom = true
    else if (stickToBottom) scheduleSync()
  }

  function onWheel(event: WheelEvent): void {
    endProgrammatic()
    if (event.deltaY < 0) stickToBottom = false
  }

  function onTouchStart(event: TouchEvent): void {
    touchY = event.touches[0]?.clientY ?? 0
  }

  function onTouchMove(event: TouchEvent): void {
    const y = event.touches[0]?.clientY ?? 0
    const dy = y - touchY
    touchY = y
    if (dy > 2) {
      endProgrammatic()
      stickToBottom = false
    }
  }

  // 指针按在滚动条上（x 超出内容宽）就是拖滚动条；点内容不算逃逸。
  function onPointerDown(event: PointerEvent): void {
    const el = scroller.value
    if (!el) return
    const rect = el.getBoundingClientRect()
    if (event.clientX - rect.left >= el.clientWidth) {
      endProgrammatic()
      stickToBottom = false
    }
  }

  function onKeyDown(event: KeyboardEvent): void {
    const target = event.target as HTMLElement | null
    if (target?.closest('input, textarea, [contenteditable="true"]')) return
    if (
      !event.defaultPrevented &&
      (['ArrowUp', 'PageUp', 'Home'].includes(event.key) || (event.key === ' ' && event.shiftKey))
    ) {
      endProgrammatic()
      stickToBottom = false
    }
  }

  /** 回到顶部 / 沉到底部：只有这里与手势能改跟随标记。 */
  function scrollToEdge(edge: 'top' | 'bottom', behavior?: ScrollBehavior): void {
    const el = scroller.value
    if (!el) return
    stickToBottom = edge === 'bottom'
    const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
    const resolved = behavior ?? (reducedMotion ? 'auto' : 'smooth')
    if (resolved !== 'smooth') {
      beginProgrammatic()
      el.scrollTo({ top: edge === 'top' ? 0 : el.scrollHeight, behavior: 'auto' })
      updateState()
      return
    }
    endProgrammatic()
    const generation = ++programmaticGeneration
    programmatic = true
    el.scrollTo({ top: edge === 'top' ? 0 : el.scrollHeight, behavior: 'smooth' })
    const settle = (): void => {
      if (generation !== programmaticGeneration) return
      clearScrollendTimer()
      programmatic = false
      updateState()
    }
    el.addEventListener('scrollend', settle, { once: true })
    removeScrollend = () => el.removeEventListener('scrollend', settle)
    scrollendTimer = window.setTimeout(() => {
      scrollendTimer = null
      settle()
    }, 400)
  }

  /** 无手势地进入跟随（打开会话、回到最新）。 */
  function stick(): void {
    stickToBottom = true
  }

  function bind(): void {
    const el = scroller.value
    if (!el) return
    resizeObserver?.disconnect()
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(scheduleSync)
      resizeObserver.observe(el)
      if (content.value) resizeObserver.observe(content.value)
    }
    el.addEventListener('scroll', onScroll)
    el.addEventListener('wheel', onWheel, { passive: true })
    el.addEventListener('touchstart', onTouchStart, { passive: true })
    el.addEventListener('touchmove', onTouchMove, { passive: true })
    el.addEventListener('pointerdown', onPointerDown)
    el.addEventListener('keydown', onKeyDown)
    syncAfterContentResize()
  }

  function unbind(): void {
    const el = scroller.value
    resizeObserver?.disconnect()
    resizeObserver = null
    if (syncFrame) {
      cancelAnimationFrame(syncFrame)
      syncFrame = 0
    }
    if (el) {
      el.removeEventListener('scroll', onScroll)
      el.removeEventListener('wheel', onWheel)
      el.removeEventListener('touchstart', onTouchStart)
      el.removeEventListener('touchmove', onTouchMove)
      el.removeEventListener('pointerdown', onPointerDown)
      el.removeEventListener('keydown', onKeyDown)
    }
    endProgrammatic()
  }

  return {
    hasOverflow,
    atTop,
    atBottom,
    scrollToEdge,
    stick,
    scheduleSync,
    bind,
    unbind,
    /** 测试缝：下一次内容长高要不要钉在最新消息上。 */
    isStuck: () => stickToBottom,
  }
}
