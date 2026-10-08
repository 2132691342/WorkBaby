import { ref } from 'vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { setBaseURL } from './api/http'

/**
 * 端口握手。
 *
 * Go 侧 OnDomReady 一触发就广播 app:ready，而 Wails 事件不缓冲给后注册的
 * 监听器——监听器晚一步，端口就永远拿不到，axios 只能拿空 baseURL 去打
 * Wails 自己的静态服务器，结果是每个接口都 404。
 *
 * 这里用**状态**而不是事件通知下游：事件在「监听器注册之前」派发同样会丢，
 * 而组件读 ref 永远读得到当前值。
 * ① 模块顶层静态注册（type=module 的脚本早于 DOMContentLoaded）
 * ② Go 侧启动后 6 秒内重复广播，前端幂等接收
 * ③ 超时兜底：绝不让用户永远停在「启动中…」
 */
export type HandshakeStatus = 'pending' | 'ready' | 'failed'

const HANDSHAKE_TIMEOUT_MS = 20000

const status = ref<HandshakeStatus>('pending')
const port = ref(0)
const message = ref('')
export { status, message }

function fail(reason: string) {
  if (status.value === 'ready') return
  status.value = 'failed'
  message.value = reason
}

function adopt(next: number) {
  if (!next || next === port.value) return
  port.value = next
  setBaseURL(next)
  if (status.value !== 'ready') {
    status.value = 'ready'
    clearTimeout(timer)
  }
}

EventsOn('app:ready', (data: { server_port?: number }) => adopt(Number(data?.server_port)))

// 后端装配失败时唯一的通知通道：启动期 HTTP 还没起来，只能靠事件。
EventsOn('app:startup-error', (data: { message?: string }) => {
  fail(data?.message || '后端启动失败，请查看日志目录里的 app.log')
})

const timer = setTimeout(() => {
  fail('本地服务没有响应。可能残留了旧版 WorkBaby 进程占用，请先在任务管理器里结束它再重试。')
}, HANDSHAKE_TIMEOUT_MS)

/** retry 让用户能立刻再试一次，不必重启整个应用。 */
export function retryHandshake() {
  if (status.value === 'ready') return
  status.value = 'pending'
  message.value = ''
  // 端口没换过就再等一轮广播；Go 侧只在启动后广播，需要重新握手只能靠后端再发一次。
  if (port.value) {
    setBaseURL(port.value)
    status.value = 'ready'
  }
}
