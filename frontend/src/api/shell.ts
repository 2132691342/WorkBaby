// 壳桥接层：window.wb 由 preload 注入，这里只包成 Promise 并留纯浏览器兜底。
// 业务一律走 HTTP，所以保持很小；字段与 electron/preload 的 EndpointVO 一致。
/** 端点信息：主进程装配好本地 HTTP 服务后回填 wb_port / wb_token。 */
export interface EndpointVO {
  wb_port: number
  wb_token: string
  wb_version: string
  wb_contract: number
  wb_base_url: string
}

// window.wb 的字段类型在 env.d.ts 里全局声明，这里不再重复。

/** 纯浏览器调试的地址覆盖。用 localStorage 显式打开，不靠「调用失败」去猜。 */
function devOverride(): EndpointVO | null {
  const port = Number(localStorage.getItem('wb.dev.port') ?? 0)
  if (!port) return null
  const token = localStorage.getItem('wb.dev.token') ?? ''
  return {
    wb_port: port,
    wb_token: token,
    wb_version: 'dev',
    wb_contract: 0,
    wb_base_url: `http://127.0.0.1:${port}/api/v1`,
  }
}

/** 拿本地服务地址。主进程装配失败时把它的原话抛上去，不吞不换。 */
export async function getEndpoint(): Promise<EndpointVO> {
  return devOverride() ?? window.wb.getEndpoint()
}

/** 打开文件选择框，可多选。返回绝对路径；取消返回空数组。 */
export async function pickFiles(title = ''): Promise<string[]> {
  try {
    return (await window.wb.pickFiles(title)) ?? []
  } catch {
    return []
  }
}

/** 打开文件夹选择框。取消返回空串，调用方本来就这么处理。 */
export async function pickFolder(defaultDir: string): Promise<string> {
  try {
    return await window.wb.pickFolder(defaultDir)
  } catch {
    // 纯浏览器调试时没有桥：返回空串等于「用户没选」。
    return ''
  }
}

/** 告诉主进程「首屏画完了，可以把窗口显出来」。 */
export async function notifyReady(): Promise<void> {
  try {
    await window.wb.notifyReady()
  } catch {
    // 纯浏览器调试时没有桥，窗口本来就已经可见，忽略即可。
  }
}

/** 用系统浏览器打开外链。 */
export async function openExternal(url: string): Promise<void> {
  try {
    await window.wb.openExternal(url)
  } catch {
    window.open(url, '_blank')
  }
}

export const windowCtl = {
  minimize: () => void window.wb.windowCtl.minimize(),
  toggleMaximize: () => void window.wb.windowCtl.toggleMaximize(),
  close: () => void window.wb.windowCtl.close(),
  /** 真的退出进程（不是收进托盘）。界面上的「退出」必须能真退。 */
  quit: () => void window.wb.windowCtl.quit(),
}

/** 执行「关窗时做什么」的选择。只由首次收托盘的对话框调用。 */
export async function applyCloseDecision(action: 'tray' | 'quit', remember: boolean): Promise<void> {
  await window.wb.applyCloseDecision(action, remember)
}
