//! preload：把壳能力以 window.wb 暴露给渲染层，业务一律走 HTTP，不在此扩面。
import { contextBridge, ipcRenderer } from 'electron'

/** 端点信息：主进程装配好本地 HTTP 服务后回填 port/token。 */
export interface EndpointVO {
  wb_port: number
  wb_token: string
  wb_version: string
  wb_contract: number
  wb_base_url: string
}

const bridge = {
  /** 拿本地服务地址；主进程装配失败时把原话抛上来。 */
  getEndpoint: (): Promise<EndpointVO> => ipcRenderer.invoke('wb:endpoint'),
  /** 打开文件选择框，可多选；取消返回空数组。 */
  pickFiles: (title: string): Promise<string[]> => ipcRenderer.invoke('wb:pick-files', title),
  /** 打开文件夹选择框；取消返回空串。 */
  pickFolder: (defaultDir: string): Promise<string> => ipcRenderer.invoke('wb:pick-folder', defaultDir),
  /** 首屏画完了，让主进程把隐藏的窗口显出来。 */
  notifyReady: (): Promise<void> => ipcRenderer.invoke('wb:ready'),
  /** 用系统浏览器打开外链，只放行 http/https。 */
  openExternal: (url: string): Promise<void> => ipcRenderer.invoke('wb:open-external', url),
  windowCtl: {
    minimize: (): Promise<void> => ipcRenderer.invoke('wb:window-ctl', 'min'),
    toggleMaximize: (): Promise<void> => ipcRenderer.invoke('wb:window-ctl', 'max'),
    close: (): Promise<void> => ipcRenderer.invoke('wb:window-ctl', 'close'),
    /** 真的退出进程；界面上的「退出」必须能真退。 */
    quit: (): Promise<void> => ipcRenderer.invoke('wb:quit'),
  },
  /** 记录「关窗时做什么」的选择；只由首次收托盘的对话框调用。 */
  applyCloseDecision: (action: 'tray' | 'quit', remember: boolean): Promise<void> =>
    ipcRenderer.invoke('wb:close-decision', action, remember),
}

contextBridge.exposeInMainWorld('wb', bridge)

export type WbBridge = typeof bridge
