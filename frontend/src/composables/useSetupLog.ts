// 安装 / 更新进度流：挂在 SSE 的全局通道（无 session_id）上，除了收集日志，
// 还把「这一趟真的结束了」变成可 await 的信号——定时器会让按钮提前说成功。
import { onBeforeUnmount, ref } from 'vue'
import { getBaseURL } from '../api/http'
import type { Envelope, SetupData } from '../types/api'

export interface SetupResult {
  ok: boolean
  /** false 表示等超时了：动作可能还在跑，也可能根本没发出来 */
  done: boolean
}

export function useSetupLog() {
  const lines = ref<string[]>([])
  const watching = ref(false)
  const phase = ref('')
  const done = ref(false)
  const ok = ref(false)
  let es: EventSource | null = null
  let waiter: ((r: SetupResult) => void) | null = null

  function settle(result: SetupResult) {
    const fn = waiter
    waiter = null
    fn?.(result)
  }

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
        const data = env.data
        if (!data) return
        // 只看自己这一路：插件更新与引擎升级共用同一条通道
        if (phase.value && data.phase && data.phase !== phase.value) return
        const msg = String(data.message || '').trim()
        if (msg && watching.value) lines.value = [...lines.value, msg].slice(-200)
        if (data.done) {
          done.value = true
          ok.value = !!data.ok
          settle({ ok: ok.value, done: true })
        }
      } catch {
        // 坏帧丢弃：进度不该因为一帧解析失败就断掉
      }
    })
  }

  /** begin 开始看某一路动作：清空旧日志、记住 phase，并确保通道已连上 */
  function begin(forPhase = '') {
    lines.value = []
    phase.value = forPhase
    done.value = false
    ok.value = false
    watching.value = true
    open()
  }

  /** whenDone 等这一趟的结束信号；先调用再接请求，避免更快的结局被漏掉。 */
  function whenDone(timeoutMs = 180000): Promise<SetupResult> {
    if (done.value) return Promise.resolve({ ok: ok.value, done: true })
    return new Promise<SetupResult>((resolve) => {
      const timer = window.setTimeout(() => {
        waiter = null
        resolve({ ok: ok.value, done: false })
      }, timeoutMs)
      waiter = (r) => {
        window.clearTimeout(timer)
        resolve(r)
      }
    })
  }

  /** end 收工：保留最后几行给用户看，停止接收新行 */
  function end() {
    watching.value = false
    settle({ ok: ok.value, done: done.value })
  }

  onBeforeUnmount(() => {
    watching.value = false
    settle({ ok: ok.value, done: done.value })
    close()
  })

  return { lines, watching, done, ok, begin, end, whenDone }
}
