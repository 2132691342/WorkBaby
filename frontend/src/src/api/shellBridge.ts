/**
 * 桌面壳 JS Bridge：Wails v2 JS runtime 不含对话框 / 桌宠绑定，本文件作为薄封装收敛调用方。
 * 浏览器预览态（未注入 window.go）抛错时返回 ok:false，调用方自行降级。
 */
import { OpenDirectoryDialog, OpenExternal, PetMove, PetToggleMode } from '@/wailsjs/go/main/App'
import { t } from '@/i18n'

export interface ShellResponse<T = unknown> {
  ok: boolean
  data?: T
  error?: string
  cancelled?: boolean
}

/**
 * 统一壳调用入口：按 action 分发给对应 Wails 绑定。
 * 浏览器预览态（未注入 window.go）抛错时返回 ok:false，调用方自行降级。
 */
export async function invokeShell<T = unknown>(action: string, params?: unknown): Promise<ShellResponse<T>> {
  try {
    switch (action) {
      case 'pet.move': {
        const p = (params ?? {}) as { dx?: number; dy?: number }
        const r = await PetMove(p.dx ?? 0, p.dy ?? 0)
        return { ok: true, data: r as unknown as T }
      }
      case 'pet.toggle': {
        const r = await PetToggleMode()
        return { ok: true, data: r as unknown as T }
      }
      default:
        return { ok: false, error: `unknown shell action: ${action}` }
    }
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) }
  }
}

/** 调原生壳目录选择器；取消或失败返回 null。 */
export async function pickDirectoryViaShell(initialPath?: string): Promise<string | null> {
  try {
    const picked = await OpenDirectoryDialog(t('ui.dialog.pickWorkspace'), initialPath ?? '')
    return picked && picked.trim() !== '' ? picked : null
  } catch {
    return null
  }
}

/**
 * 归一化待打开的链接：仅放行 http(s)/file/mailto 与根相对路径。
 *
 * <p>根相对路径（如 {@code /files/...} 托管资源）拼 WebView origin 后
 * 交给系统浏览器，Wails AssetServer 可直接服务。其余形式（相对页内链接、
 * 非法协议）返回空串直接忽略，绝不带入系统浏览器。
 */
export function normalizeExternalUrl(url: string): string {
  const u = (url ?? '').trim()
  if (!u || u.startsWith('#')) return ''
  if (/^(https?|file|mailto):/i.test(u)) return u
  if (u.startsWith('/')) return window.location.origin + u
  return ''
}

/**
 * 用系统默认浏览器打开链接（绝不触发 WebView 内导航）。
 *
 * <p>聊天正文 / 产物卡 / 工作区面板的链接点击一律走这里：WebView 内导航会把
 * 整个 SPA 页面替换掉，应用随之假死。浏览器预览态（无 Wails 注入）静默忽略。
 */
export async function openExternal(url: string): Promise<void> {
  const target = normalizeExternalUrl(url)
  if (!target) return
  try {
    await OpenExternal(target)
  } catch {
    // 无 Wails 注入（纯浏览器 dev）：退回新标签页打开，不影响 SPA 本体
    window.open(target, '_blank', 'noopener')
  }
}
