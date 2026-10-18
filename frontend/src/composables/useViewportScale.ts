// 视口比例缩放：按「窗口宽 ÷ 基准宽」写 --wb-vw-scale，全套尺寸令牌等比跟随。
// 分辨率不同的机器不改代码：1080p 缩小、2K/4K 放大，组件层全部走 var(--wb-*)。
import { onBeforeUnmount, onMounted } from 'vue'

// 基准设计宽：所有刻度令牌按这个宽度定标。
const BASE_WIDTH = 1440
// 缩放上下限：太小会挤破布局（窗口最窄 960px ≈ 0.67），太大会虚胖。
// 下限取 0.8：最窄窗口时正文仍可读，剩余差额交给各视图自己的响应式断点。
const MIN_SCALE = 0.8
const MAX_SCALE = 1.35

let raf = 0

function applyScale() {
  raf = 0
  // 用 visualViewport：缩放窗口时它先于 resize 生效，且不受浏览器缩放干扰
  const w = window.visualViewport?.width ?? window.innerWidth
  const scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, w / BASE_WIDTH))
  document.documentElement.style.setProperty('--wb-vw-scale', scale.toFixed(4))
}

function queueApply() {
  if (!raf) raf = window.requestAnimationFrame(applyScale)
}

// 应用壳只挂一次：重复挂载只会多挂几份监听器。
export function useViewportScale() {
  onMounted(() => {
    applyScale()
    window.addEventListener('resize', queueApply, { passive: true })
    window.visualViewport?.addEventListener('resize', queueApply, { passive: true })
  })
  onBeforeUnmount(() => {
    window.removeEventListener('resize', queueApply)
    window.visualViewport?.removeEventListener('resize', queueApply)
    if (raf) cancelAnimationFrame(raf)
    raf = 0
  })
}
