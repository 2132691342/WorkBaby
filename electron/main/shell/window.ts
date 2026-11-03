//! 主窗口：无边框 1440×900，开发态加载 Vite 热更，产物态加载前端 dist。
import { BrowserWindow } from 'electron'
import path from 'node:path'
import fs from 'node:fs'

let main: BrowserWindow | null = null

const DEV_URL = 'http://127.0.0.1:5173'
const MIN_WIDTH = 960
const MIN_HEIGHT = 640
const WIDTH = 1440
const HEIGHT = 900

/** 拿主窗口；没有就建（托盘/second-instance 都可能先于 ready 后首次调用）。 */
export function createMainWindow(): BrowserWindow {
  if (main && !main.isDestroyed()) return main
  main = new BrowserWindow({
    width: WIDTH,
    height: HEIGHT,
    minWidth: MIN_WIDTH,
    minHeight: MIN_HEIGHT,
    frame: false,
    show: false,
    backgroundColor: '#f8fafc',
    webPreferences: {
      preload: path.join(__dirname, '../preload/index.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  main.once('ready-to-show', () => main?.show())
  main.on('closed', () => {
    main = null
  })

  if (process.env.WB_DEV === '1') {
    void main.loadURL(DEV_URL)
  } else {
    loadBundledFrontend(main)
  }
  return main
}

/** 取现有主窗口；没有返回 null（托盘点击、对话框父窗等场景）。 */
export function getMainWindow(): BrowserWindow | null {
  return main && !main.isDestroyed() ? main : null
}

/** 加载打包进安装包的前端；缺失时给占位页，保证壳本身永远能开。 */
function loadBundledFrontend(win: BrowserWindow): void {
  const index = path.join(__dirname, '../../frontend/dist/index.html')
  if (fs.existsSync(index)) {
    void win.loadFile(index)
  } else {
    void win.loadURL(
      'data:text/html;charset=utf-8,' +
        encodeURIComponent('<body style="font-family:sans-serif;background:#f8fafc;color:#1e293b;display:grid;place-items:center;height:100vh;margin:0"><h1>WorkBaby</h1></body>'),
    )
  }
}
