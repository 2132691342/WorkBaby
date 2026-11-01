// 壳层调用唯一出口：Tauri 命令与壳事件的收口。组件不得直接 import @tauri-apps/*。
// 浏览器直开（无 Tauri）时，端口握手读 localStorage 的 wb_dev_port 兜底。

/** 是否运行在 Tauri 壳内：注入脚本存在即可用 invoke。 */
export const isTauri =
  typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window

type ShellEvent =
  | 'app:ready'
  | 'app:startup-error'
  | 'app:to-tray'
  | 'app:new-session'
  | 'app:open-file'

async function invoke<T>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  if (!isTauri) throw new Error('invoke requires the desktop shell')
  const { invoke: call } = await import('@tauri-apps/api/core')
  return call<T>(cmd, args)
}

/** 拉取本地 HTTP 服务端口（幂等；握手失败时 retryHandshake 会重调）。 */
export async function serverPort(): Promise<number> {
  if (isTauri) return invoke<number>('server_port')
  return Number(localStorage.getItem('wb_dev_port') || 0)
}

/** 订阅壳事件；返回解绑函数。 */
export async function onShellEvent(
  name: ShellEvent,
  handler: (data: Record<string, unknown>) => void,
): Promise<() => void> {
  if (!isTauri) return () => undefined
  const { listen } = await import('@tauri-apps/api/event')
  const un = await listen<Record<string, unknown>>(name, (e) => handler(e.payload))
  return () => un()
}

/** 文件选择：返回所选路径，取消返回 null。pattern 形如 '*.png;*.jpg'。 */
export async function pickFile(title?: string, pattern?: string): Promise<string | null> {
  return invoke<string | null>('pick_file', { title: title ?? '', pattern: pattern ?? '' })
}

/** 另存为：返回目标路径，取消返回 null。 */
export async function pickFileSave(
  title: string,
  defaultName: string,
  pattern: string,
): Promise<string | null> {
  return invoke<string | null>('pick_file_save', { title, defaultName, pattern })
}

/** 目录选择：返回所选目录，取消返回 null。 */
export async function pickFolder(title?: string): Promise<string | null> {
  return invoke<string | null>('pick_folder', { title: title ?? '' })
}

/** 目录选择（可选）：非桌面环境返回 null，不抛错——微前端页可降级为手填路径。 */
export async function pickFolderSafe(title?: string): Promise<string | null> {
  if (!isTauri) return null
  try {
    return await pickFolder(title)
  } catch {
    return null
  }
}

/** 开机自启状态与开关（HKCU，不需要管理员）。 */
export async function autostartGet(): Promise<boolean> {
  return invoke<boolean>('autostart_get')
}

export async function autostartSet(on: boolean): Promise<void> {
  return invoke<void>('autostart_set', { on })
}

/** 真退出：绕过关窗到托盘的策略。 */
export function quitApp(): void {
  void invoke<void>('quit_app').catch(() => window.close())
}

export type WindowAction = 'min' | 'max' | 'close'

/** 窗控三键：close 走后端，由关窗策略决定收托盘还是退出。 */
export function windowCtl(action: WindowAction): void {
  void invoke<void>('window_ctl', { action }).catch(() => undefined)
}
