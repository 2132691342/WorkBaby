//! preload 桥的主进程侧：对话框、窗口控制、外链与端点分发。
import { app, dialog, ipcMain, shell } from 'electron'
import { getMainWindow } from './window'

type CloseAction = 'tray' | 'quit'

// 关闭策略：默认收托盘；「记住选择」在 phase 2 落到设置库。
let close_action: CloseAction = 'tray'

/** 注册 window.wb 全部 IPC 通道；频道名即 preload 合同，不许改。 */
export function registerBridge(): void {
  ipcMain.handle('wb:endpoint', () => ({
    wb_port: 0,
    wb_token: '',
    wb_version: app.getVersion(),
    wb_contract: 1,
    wb_base_url: '',
  }))

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

  ipcMain.handle('wb:close-decision', (_e, action: CloseAction, remember: boolean) => {
    close_action = action
    if (remember) persistCloseAction(action)
  })
}

function toggleMaximize(win: BrowserWindow): void {
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

function persistCloseAction(_action: CloseAction): void {
  // phase 2 接设置库后写入持久层；内存值在进程内已生效。
}

/** 主进程其他模块读关闭策略用。 */
export function getCloseAction(): CloseAction {
  return close_action
}
