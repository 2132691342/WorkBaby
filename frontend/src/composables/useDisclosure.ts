// 折叠披露：显式选择的会话级记忆 + 阅读保护。
// 只记用户显式动过的开合，没动过的走派生默认；显式选择跨驱逐（活动段重排、
// 收尾换历史渲染造成的重挂）保留——用户展开过的卡再见面时还开着。
import { computed, onScopeDispose, ref, watch, type Ref, type WritableComputedRef } from 'vue'
import { useChatStore } from '@/stores/chat'

// 显式选择表：identity → 用户选的开合。键带会话前缀，不落盘、不跨会话串味。
const choices = new Map<string, boolean>()

// 恒关的派生默认：没有自动展开逻辑的披露用它。
const CLOSED = ref(false)

/**
 * 一处披露的开合。写入 open 即记录显式选择（自动逻辑只改 autoOpen，
 * 不留痕）；显式选择永远压过派生默认。
 */
export function useDisclosure(
  identity: Ref<string>,
  autoOpen: Ref<boolean> = CLOSED,
): { open: WritableComputedRef<boolean> } {
  const chat = useChatStore()
  const chosen = ref<boolean | undefined>(undefined)

  const key = (): string => (identity.value ? `${chat.sessionFile}|${identity.value}` : '')

  // 挂载即回放记忆：驱逐重挂后，显式选择不该被自动折叠吃掉。
  const initial = key()
  if (initial) chosen.value = choices.get(initial)

  // 身份后到（占位块补上真身）同样接得上记忆。
  watch(identity, () => {
    const k = key()
    if (k && chosen.value === undefined) chosen.value = choices.get(k)
  })

  const open = computed<boolean>({
    get: () => (chosen.value !== undefined ? chosen.value : autoOpen.value),
    set: (next: boolean) => {
      chosen.value = next
      const k = key()
      if (k) choices.set(k, next)
    },
  })

  return { open }
}

// 阅读中枢：全部披露共用一组文档级监听，每处只重查自己的 DOM——
// 焦点或选区变化的代价是几次 contains，不是每层一对监听。
const readingHolds = new Set<() => void>()
let hubInstalled = false

function installReadingHub(): void {
  if (hubInstalled || typeof document === 'undefined') return
  hubInstalled = true
  const syncAll = (): void => {
    for (const sync of [...readingHolds]) sync()
  }
  document.addEventListener('focusin', syncAll)
  document.addEventListener('selectionchange', syncAll)
  window.addEventListener('pointerup', syncAll)
}

// 焦点或文本选区落在层内就算「正在读」：键盘焦点在标题上、选了一段输出，
// 都不能被自动折叠收走。选择必须是活的（未塌缩），否则处处都是阅读保护。
function ownsReadingPosition(level: HTMLElement): boolean {
  const active = document.activeElement
  if (active && level.contains(active)) return true
  const selection = window.getSelection()
  return Boolean(
    selection &&
      !selection.isCollapsed &&
      (level.contains(selection.anchorNode) || level.contains(selection.focusNode)),
  )
}

/**
 * 阅读保护（瞬时状态，不是披露选择）：为真时自动折叠必须让路。
 * 人一离开（焦点走、选区散）立刻交还给自动逻辑，不留下任何记忆。
 */
export function useReadingHold(el: Ref<HTMLElement | null>): Ref<boolean> {
  const reading = ref(false)
  const sync = (): void => {
    const level = el.value
    reading.value = level ? ownsReadingPosition(level) : false
  }
  installReadingHub()
  readingHolds.add(sync)
  onScopeDispose(() => readingHolds.delete(sync))
  return reading
}
