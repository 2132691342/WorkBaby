//! WorkBaby 壳入口：上下文 → 日志 → DB → HTTP → 托盘 → 窗口，重活后台跑。
import { app } from 'electron'
import path from 'node:path'
import { initContext, paths, setPort } from './context'
import { initLog, closeLog, logInfo, logError } from './pkg/log'
import { openDb, closeDb } from './repo/db'
import { buildServer, startServer } from './api/server'
import { provisionRuntime } from './service/engine/runtime'
import { provisionSkills } from './service/skill_service'
import { declareBuiltinPlugins, provisionBuiltinPlugins } from './service/plugin_service'
import { ensureNpmCommand } from './service/plugin_settings'
import { piInstalled } from './service/engine/runtime'
import { chatRestart } from './service/chat/chat_service'
import { registerBridge, setEndpointPort } from './shell/bridge'
import { createMainWindow } from './shell/window'
import { createTray, destroyTray } from './shell/tray'

// 单实例：二次启动把已有窗口带到前台，不另起进程。
const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  app.quit()
} else {
  initContext()
  initLog(paths().wb_log_dir)
  openDb(paths().wb_db_path)

  app.on('second-instance', () => {
    const win = createMainWindow()
    if (win.isMinimized()) win.restore()
    win.focus()
  })

  app.whenReady().then(() => {
    registerBridge()
    createMainWindow()
    createTray()
    void bootstrap()
  })

  // Windows 常规退出路径：所有窗口关闭即退出（关窗默认收托盘，另有开关）。
  app.on('window-all-closed', () => {
    app.quit()
  })

  app.on('before-quit', () => {
    try {
      chatRestart()
      closeDb()
      closeLog()
    } catch (err) {
      logError(`收尾失败：${err instanceof Error ? err.message : String(err)}`)
    } finally {
      destroyTray()
    }
  })
}

/**
 * 装配：起 HTTP → 回填端口；运行时解压与技能落位并行后台跑。
 * 运行时解压完才能把 settings.json 的 npmCommand 指到内置 npm，
 * 所以插件声明（不装包）先落，装包放到运行时之后。
 */
async function bootstrap(): Promise<void> {
  try {
    const port = await startServer(buildServer())
    setPort(port)
    setEndpointPort(port)
    logInfo(`HTTP 服务就绪：127.0.0.1:${port}`)
  } catch (err) {
    logError(`HTTP 服务启动失败：${err instanceof Error ? err.message : String(err)}`)
    return
  }
  try {
    await provisionRuntime()
    provisionSkills()
    // 声明与 npmCommand 都指向沙箱，先写好；pi 没装时 packages 声明不影响任何东西。
    ensureNpmCommand()
    declareBuiltinPlugins()
    logInfo(`运行时与技能就绪（数据目录 ${path.basename(paths().wb_data_dir)}）`)
  } catch (err) {
    logError(`运行时引导失败：${err instanceof Error ? err.message : String(err)}`)
    return
  }
  if (!piInstalled()) {
    logInfo('pi 尚未安装，内置插件等 pi 就绪后再装')
    return
  }
  try {
    ensureNpmCommand()
    const result = await provisionBuiltinPlugins()
    logInfo(`内置插件就绪：新增 ${result.installed}，失败 ${result.failed}`)
  } catch (err) {
    // 内置插件装不上不影响开聊；界面上每行有重试。
    logError(`内置插件安装失败：${err instanceof Error ? err.message : String(err)}`)
  }
}
