//! 托盘：最小菜单（显示 / 退出），关窗收托盘时的唯一入口。
import { app, Menu, Tray, nativeImage } from 'electron'
import { getMainWindow } from './window'

// 16×16 的 wb 图标（启动画面同款配色），内联避免资源装载顺序问题。
const ICON_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAMElEQVR4nGNITvvIQAmmSDO6Af9JxMPagB+//oAxLj7tDRg8YYCO6WfAgIXBwORGAFkLYZ/utodCAAAAAElFTkSuQmCC'

let tray: Tray | null = null

/** 建托盘；已有就复用。 */
export function createTray(): void {
  if (tray) return
  tray = new Tray(nativeImage.createFromBuffer(Buffer.from(ICON_BASE64, 'base64')))
  tray.setToolTip('WorkBaby')
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: '显示主窗口', click: () => showMain() },
      { type: 'separator' },
      { label: '退出 WorkBaby', click: () => app.quit() },
    ]),
  )
  tray.on('click', () => showMain())
}

function showMain(): void {
  const win = getMainWindow()
  if (!win) return
  if (win.isMinimized()) win.restore()
  win.show()
  win.focus()
}

/** 退出前销毁托盘，避免残留图标。 */
export function destroyTray(): void {
  tray?.destroy()
  tray = null
}
