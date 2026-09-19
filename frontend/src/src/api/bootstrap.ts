import { initApi, apiGet } from './http'
import { UI_API_CONTRACT_VERSION } from './contract'

/**
 * 双主机启动引导：取 gin 监听端口并初始化 HTTP 层，随后校验契约版本。
 * 取端口依次尝试 Wails 绑定、app:ready 事件、localStorage 缓存；全部失败即启动失败。
 */
export async function bootstrapServer(): Promise<void> {
  const port = await acquirePort()
  if (port <= 0) {
    throw new Error('server not ready: missing serverPort')
  }
  initApi(port)
  await checkContract()
}

/** 端口获取总入口：绑定 → 事件 / 轮询竞速 → localStorage，全部失败返回 0。 */
async function acquirePort(): Promise<number> {
  const bound = await readPortBinding()
  if (bound > 0) return bound

  const fromEvent = await waitReadyEvent()
  if (fromEvent > 0) return fromEvent

  const cached = readCachedPort()
  return cached
}

/** 经 Wails 绑定同步取端口；非桌面环境（纯浏览器 dev）返回 0。 */
async function readPortBinding(): Promise<number> {
  try {
    const { GetServerPort } = await import('@/wailsjs/go/main/App')
    const port = await GetServerPort()
    return typeof port === 'number' && port > 0 ? port : 0
  } catch {
    return 0
  }
}

/**
 * 等 `app:ready` 事件；同时以 300ms 间隔轮询绑定，覆盖「事件已经先发过」的情况。
 * 超时后返回 0，由调用方决定降级，不在这里吞掉失败。
 */
function waitReadyEvent(timeoutMs = 15000): Promise<number> {
  return new Promise<number>((resolve) => {
    let settled = false
    let off: (() => void) | null = null
    const finish = (port: number): void => {
      if (settled) return
      settled = true
      off?.()
      if (timer !== null) clearInterval(timer)
      resolve(port)
    }

    const deadline = Date.now() + timeoutMs
    const timer = setInterval(async () => {
      const port = await readPortBinding()
      if (port > 0) {
        finish(port)
        return
      }
      if (Date.now() > deadline) finish(0)
    }, 300)

    void import('@/wailsjs/runtime/runtime')
      .then((rt) => {
        off = rt.EventsOn('app:ready', (payload: Record<string, unknown>) => {
          const port = payload?.server_port as number | undefined
          if (typeof port === 'number' && port > 0) finish(port)
        })
      })
      .catch(() => {
        // 纯浏览器 dev：没有 runtime，只能等轮询超时
      })
  })
}

/** 读上一次运行缓存的端口（可能已失效，仅在非桌面环境兜底）。 */
function readCachedPort(): number {
  try {
    const cached = localStorage.getItem('wb.serverPort')
    const n = Number(cached)
    return Number.isFinite(n) && n > 0 ? n : 0
  } catch {
    return 0
  }
}

/** 契约版本比对：后端 meta/contract 与前端常量不一致时显式抛错，避免全站静默 404。 */
async function checkContract(): Promise<void> {
  let server = UI_API_CONTRACT_VERSION
  try {
    server = await apiGet<number>('/api/v1/meta/contract')
  } catch {
    return // 后端不可达由后续业务调用暴露，此处不阻断
  }
  if (server !== UI_API_CONTRACT_VERSION) {
    throw new Error(
      `[contract] 前后端契约版本不一致：后端 v${server} / 前端 v${UI_API_CONTRACT_VERSION}。请同步升级后重启。`
    )
  }
}
