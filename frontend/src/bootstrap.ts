// 启动装配：桌面壳端口握手与引导数据加载。
import { ref } from 'vue'
import { ServerPort } from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { setBaseURL } from './api/http'

/** 端口握手：拿到本地 HTTP 端口之前，所有业务接口都还没有 baseURL（会直接 404）。
 *  两条路同时走——Go 侧启动后广播一次 app:ready；前端自己再拉 ServerPort 绑定方法。
 *  只广播一次是够的：晚注册的监听器可以用拉的方式补上，端口不变，重复拉取幂等。 */
export type HandshakeStatus = 'pending' | 'ready' | 'failed'

const HANDSHAKE_TIMEOUT_MS = 20000
// 拉取节奏：装配期要解压内置运行时，端口可能晚几秒才出现。
const PULL_INTERVAL_MS = 400

const status = ref<HandshakeStatus>('pending')
// 加载页的阶段读数：握手本地服务 → 读取引导数据 → 检查助手环境 → 完成。
export type BootStage = 'handshake' | 'bootstrap' | 'env' | 'done'
const stage = ref<BootStage>('handshake')
const port = ref(0)
const message = ref('')
export { status, stage, message }

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
    stage.value = 'bootstrap'
    clearTimeout(timer)
    stopPull()
  }
}

let timer = 0
let puller: ReturnType<typeof setInterval> | undefined

/** 主动拉一次端口：绑定方法在 WebView 就绪后随时可调，等 window.go 出现前会抛，忽略即可。 */
function pullPort() {
  if (status.value === 'ready') return stopPull()
  try {
    Promise.resolve(ServerPort())
      .then((p) => adopt(Number(p) || 0))
      .catch(() => undefined)
  } catch {
    // 绑定面还没挂上：下一轮再试。
  }
}

function startPull() {
  if (puller !== undefined) return
  pullPort()
  puller = setInterval(pullPort, PULL_INTERVAL_MS)
}

function stopPull() {
  if (puller === undefined) return
  clearInterval(puller)
  puller = undefined
}

function armTimeout() {
  clearTimeout(timer)
  timer = setTimeout(() => {
    fail('本地服务没有响应。可能残留了旧版 WorkBaby 进程占用，请先在任务管理器里结束它再重试。')
  }, HANDSHAKE_TIMEOUT_MS) as unknown as number
}

EventsOn('app:ready', (data: { server_port?: number }) => adopt(Number(data?.server_port)))

// 后端装配失败时唯一的通知通道：启动期 HTTP 还没起来，只能靠事件。
EventsOn('app:startup-error', (data: { message?: string }) => {
  stopPull()
  fail(data?.message || '后端启动失败，请查看日志目录里的 app.log')
})

armTimeout()
startPull()

/** retryHandshake 让用户能立刻再试一次，不必重启整个应用。 */
export function retryHandshake() {
  if (status.value === 'ready') return
  status.value = 'pending'
  message.value = ''
  // 端口已经拿到过（只是某次请求失败）：直接用，不必再等一轮。
  if (port.value) {
    setBaseURL(port.value)
    status.value = 'ready'
    return
  }
  // 再拉一轮必须重挂超时，否则再点一次重试就永远停在「正在启动…」。
  armTimeout()
  startPull()
}
