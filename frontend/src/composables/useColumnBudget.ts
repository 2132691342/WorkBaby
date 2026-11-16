// 三栏预算制：窗口再窄，主区（消息流 + 输入区）也不能跌破硬底——
// 预算不够时右工作台先收窄、收不下就折叠，侧栏的收窄 / 折叠交给容器查询。
// 硬底按 vw-scale 折算：刻度缩小时输入区排布也缩，地板跟着降才不误杀。
import { onMounted, onUnmounted, type Ref } from 'vue'
import { vwScale } from '@/composables/useViewportScale'
import { useWorkPanel } from '@/composables/useWorkPanel'

/** 主区硬底（刻度 1 时的像素）：再窄输入区工具行就折行了。 */
const MAIN_FLOOR = 450

/**
 * 把「页面宽 − 侧栏宽 − 主区硬底」作为右工作台的实时预算喂给面板。
 * 页面根与侧栏都要量：侧栏在窄档会收窄甚至折叠，预算必须跟着变。
 */
export function useColumnBudget(page: Ref<HTMLElement | null>, side: Ref<HTMLElement | null>): void {
  const panel = useWorkPanel()
  let ro: ResizeObserver | null = null

  function measure(): void {
    const pageEl = page.value
    if (!pageEl) return
    const sideWidth = side.value?.offsetWidth ?? 0
    panel.setBudget(Math.round(pageEl.clientWidth - sideWidth - MAIN_FLOOR * vwScale.value))
  }

  onMounted(() => {
    if (typeof ResizeObserver !== 'undefined') {
      ro = new ResizeObserver(measure)
      if (page.value) ro.observe(page.value)
      if (side.value) ro.observe(side.value)
    }
    measure()
  })
  onUnmounted(() => {
    ro?.disconnect()
    ro = null
  })
}
