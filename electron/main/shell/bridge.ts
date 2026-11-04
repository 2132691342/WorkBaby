//! preload 桥的主进程侧：对话框、窗口控制、外链与端点分发。
import { app, dialog, ipcMain, shell } from 'electron'
import { getMainWindow } from './window'
import { token, paths } from '../context'
import { prefsCloseAction, prefsSetCloseAction } from '../service/settings_service'

/** 注册 window.wb 全部 IPC 通道；频道名即 preload 合同，不许改。 */
export function registerBridge(): void {
  ipcMain.handle('wb:endpoint', () => {
    const port = contextPort
    return {
      wb_port: port,
      wb_token: token(),
      wb_version: app.getVersion(),
      wb_contract: 1,
      wb_base_url: port > 0 ? `http://127.0.0.1:${port}/api/v1` : '',
    }
  })

  ipcMain.handle('wb:pick-files', async (_e, title: string) => {
    const result = await dialog.showOpenDialog({
      title: title || '选择文件',
      properties: ['openFile', 'multiSelections'],
    })
    return result.canceled ? [] : result.filePaths
  })

  ipcMain.handle('wb:pick-folder', async (_e, defaultDir: string) => {
    const result = await dialog.showOpenDialog({
      title: '选择文件夹',
      defaultPath: defaultDir || undefined,
      properties: ['openDirectory'],
    })
    return result.canceled || result.filePaths.length === 0 ? '' : result.filePaths[0]
  })

  ipcMain.handle('wb:ready', () => {
    const win = getMainWindow()
    if (win && !win.isVisible()) win.show()
  })

  ipcMain.handle('wb:open-external', (_e, url: string) => {
    if (/^https?:\/\//i.test(url)) void shell.openExternal(url)
  })

  ipcMain.handle('wb:window-ctl', (_e, action: 'min' | 'max' | 'close') => {
    const win = getMainWindow()
    if (!win) return
    if (action === 'min') win.minimize()
    else if (action === 'max') toggleMaximize(win)
    else applyClose()
  })

  ipcMain.handle('wb:quit', () => app.quit())

  ipcMain.handle('wb:close-decision', (_e, action: 'tray' | 'quit', remember: boolean) => {
    close_action = action
    if (remember) prefsSetCloseAction(action)
  })

  // 启动时恢复用户上次选择的关窗行为。
  close_action = prefsCloseAction()
  void paths()
}

/** HTTP 服务起好后回填端口；前端在装配完成前拿到的 base_url 为空会等待。 */
let contextPort = 0

/** 装配根把端口送进来。 */
export function setEndpointPort(port: number): void {
  contextPort = port
}

let close_action: 'tray' | 'quit' = 'tray'

function toggleMaximize(win: Electron.BrowserWindow): void {
  if (win.isMaximized()) win.unmaximize()
  else win.maximize()
}

/** 关闭按钮统一走这里：收托盘或真退出，按用户选择。 */
function applyClose(): void {
  const win = getMainWindow()
  if (close_action === 'tray' && win) {
    win.hide()
  } else {
    app.quit()
  }
}

/** 主进程其他模块读关闭策略用。 */
export function getCloseAction(): 'tray' | 'quit' {
  return close_action
}
