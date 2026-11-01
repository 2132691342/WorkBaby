// 启动装配：桌面壳端口握手与引导数据加载。
import { ref } from 'vue'
import { isTauri, onShellEvent, serverPort } from './api/shell'
import { setBaseURL } from './api/http'
import { i18n } from './i18n'

/** 端口握手：拿到本地 HTTP 端口之前，所有业务接口都还没有 baseURL（会直接 404）。
 *  两条路同时走——壳启动后广播一次 app:ready；前端自己再拉一次 server_port 命令。
 *  只广播一次是够的：晚注册的监听器可以用拉的方式补上，端口不变，重复拉取幂等。 */
export type HandshakeStatus = 'pending' | 'ready' | 'failed'

const HANDSHAKE_TIMEOUT_MS = 20000

const status = ref<HandshakeStatus>('pending')
// 加载页的阶段读数：握手本地服务 → 读取引导数据 → 完成。
export type BootStage = 'handshake' | 'bootstrap' | 'done'
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
  }
}

let timer: ReturnType<typeof setTimeout> | undefined

async function pullPort() {
  if (status.value === 'ready') return
  try {
    adopt(Number(await serverPort()) || 0)
  } catch {
    // 壳命令面还没就绪：事件到达或下一轮重试再补。
  }
}

function armTimeout() {
  clearTimeout(timer)
  timer = setTimeout(() => {
    const t = i18n.global.t
    fail(isTauri ? t('boot.noService') : t('boot.noShell'))
  }, HANDSHAKE_TIMEOUT_MS)
}

void onShellEvent('app:ready', (data) => adopt(Number(data?.server_port)))
// 后端装配失败时唯一的通知通道：启动期 HTTP 还没起来，只能靠事件。
void onShellEvent('app:startup-error', (data) => {
  fail(String(data?.message || i18n.global.t('boot.startupFailed')))
})

armTimeout()
void pullPort()

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
  armTimeout()
  void pullPort()
}
