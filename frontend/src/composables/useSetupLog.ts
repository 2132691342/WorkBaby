// 安装进度流：环境与插件的安装挂在 SSE 的全局通道（session_id 为空）上。
// 它们发生在任何会话之前，也发生在会话之外，绑某个会话就收不到。
import { onBeforeUnmount, ref } from 'vue'
import { getBaseURL } from '../api/http'
import type { Envelope, SetupData } from '../types/api'

export function useSetupLog() {
  const lines = ref<string[]>([])
  const watching = ref(false)
  let es: EventSource | null = null

  function close() {
    if (es) {
      es.close()
      es = null
    }
  }

  function open() {
    const base = getBaseURL()
    if (!base || es) return
    es = new EventSource(`${base}/events?session_id=`)
    es.addEventListener('chat:setup', (ev) => {
      try {
        const env = JSON.parse((ev as MessageEvent).data) as Envelope<SetupData>
        const msg = String(env.data?.message || '').trim()
        if (!msg) return
        // 只收自己关心的那几条：完整日志留给写日志的人，界面只显示这次动作
        if (watching.value) lines.value = [...lines.value, msg].slice(-200)
      } catch {
        // 坏帧丢弃：进度不该因为一帧解析失败就断掉
      }
    })
  }

  /** begin 开始看某一路安装：清空旧日志并确保通道已连上 */
  function begin() {
    lines.value = []
    watching.value = true
    open()
  }

  /** end 收工：保留最后几行给用户看，停止接收新行 */
  function end() {
    watching.value = false
  }

  onBeforeUnmount(() => {
    watching.value = false
    close()
  })

  return { lines, watching, begin, end }
}
