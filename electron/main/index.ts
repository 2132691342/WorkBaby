//! WorkBaby 壳入口：单实例、主窗口、preload 桥与生命周期。
import { app } from 'electron'
import { createMainWindow } from './shell/window'
import { registerBridge } from './shell/bridge'

// 单实例：二次启动把已有窗口带到前台，不另起进程。
const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  app.quit()
} else {
  app.on('second-instance', () => {
    const win = createMainWindow()
    if (win.isMinimized()) win.restore()
    win.focus()
  })

  app.whenReady().then(() => {
    registerBridge()
    createMainWindow()
  })

  // Windows 常规退出路径：所有窗口关闭即退出（托盘策略后续接在 close 事件上）。
  app.on('window-all-closed', () => {
    app.quit()
  })
}
