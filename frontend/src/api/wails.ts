// Wails 绑定层：拿本地服务地址与令牌、系统对话框、窗口控制。
// v3 的绑定由 `wails3 generate bindings` 静态生成，这里只把调用包成 Promise，
// 并保留「纯浏览器调后端」的兜底；业务一律走 HTTP，所以保持很小。
import * as App from '@wails/WorkBaby/backend/shell/app'
import type { Endpoint } from '@wails/WorkBaby/backend/shell/models'

export type { Endpoint }

/**
 * 纯浏览器调试的地址覆盖。用 localStorage 显式打开，不靠「调用失败」去猜。
 *
 * 为什么不用异常来判断：后端装配失败时绑定调用也会抛错，两者混在一起就只能
 * 猜。猜错的表现是把「引擎配置读不了」写成「没有 Wails 绑定」——
 * 用户拿着错误的提示去排查，永远修不好。
 */
function devOverride(): Endpoint | null {
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

/**
 * 取本地服务地址。后端装配失败时把它的原话抛上去，不吞不换。
 *
 * 装配失败的原因（数据目录不可写、端口占用、设置库读不了）只有后端知道，
 * 前端唯一的职责是把它完整显示出来。
 */
export async function getEndpoint(): Promise<Endpoint> {
  return devOverride() ?? App.Endpoint()
}

/** 打开文件选择框，可多选。返回绝对路径；取消返回空数组。 */
export async function pickFiles(title = ''): Promise<string[]> {
  try {
    return (await App.PickFiles(title)) ?? []
  } catch {
    return []
  }
}

/** 打开文件夹选择框。取消返回空串，调用方本来就这么处理。 */
export async function pickFolder(defaultDir: string): Promise<string> {
  try {
    return await App.PickFolder(defaultDir)
  } catch {
    // 纯浏览器调试时没有对话框：返回空串等于「用户没选」。
    return ''
  }
}

/**
 * 告诉后端「首屏画完了，可以把窗口显出来」。
 *
 * 窗口在装配阶段就建好了，但那时还是隐藏的：v3 的 Show() 在窗口尚未创建完时
 * 只会建窗而不显形。等这里调用再显示，用户第一眼看到的就是画好的界面。
 */
export async function notifyReady(): Promise<void> {
  try {
    await App.Ready()
  } catch {
    // 纯浏览器调试时没有绑定，窗口本来就已经可见，忽略即可。
  }
}

/** 用系统浏览器打开外链。 */
export async function openExternal(url: string): Promise<void> {
  try {
    await App.OpenExternal(url)
  } catch {
    window.open(url, '_blank')
  }
}

export const windowCtl = {
  minimize: () => void App.WindowMinimize(),
  toggleMaximize: () => void App.WindowToggleMaximize(),
  close: () => void App.WindowClose(),
  /** 真的退出进程（不是收进托盘）。界面上的「退出」必须能真退。 */
  quit: () => void App.Quit(),
}

/**
 * 执行「关窗时做什么」的选择。只由首次收托盘的对话框调用。
 *
 * 为什么需要它：关闭按钮第一次被按时，后端不知道用户是想收进托盘还是
 * 想关掉——两个都合理。与其猜，不如问一次并把选择记住。
 */
export async function applyCloseDecision(action: 'tray' | 'quit', remember: boolean): Promise<void> {
  await App.ApplyCloseDecision(action, remember)
}
