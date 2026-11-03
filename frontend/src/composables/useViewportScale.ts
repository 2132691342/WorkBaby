// 视口比例缩放：把「窗口宽 ÷ 1440」写进 --wb-vw-scale。
// 组件全部走 --wb-* 令牌，改这一个值就够，不必为每个断点写媒体查询；
// 上下限锁死 0.75~1.35：再小字没法读，再大就是放大模式。
import { onMounted, onUnmounted } from 'vue'

const BASE_WIDTH = 1440
const MIN_SCALE = 0.75
const MAX_SCALE = 1.35

function apply() {
  const scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, window.innerWidth / BASE_WIDTH))
  document.documentElement.style.setProperty('--wb-vw-scale', String(Number(scale.toFixed(3))))
}

export function useViewportScale() {
  let timer: number | undefined
  const onResize = () => {
    window.clearTimeout(timer)
    timer = window.setTimeout(apply, 80)
  }
  onMounted(() => {
    apply()
    window.addEventListener('resize', onResize)
  })
  onUnmounted(() => {
    window.clearTimeout(timer)
    window.removeEventListener('resize', onResize)
  })
}
