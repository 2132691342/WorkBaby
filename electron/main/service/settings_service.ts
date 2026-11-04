//! 设置与偏好：KV 存储、工作区、应用偏好与引擎配置原文。
import { app, shell } from 'electron'
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import {
  forgetWorkspace,
  getAllSettings,
  getSetting,
  getWorkspace,
  saveSettings,
  setWorkspace,
} from '../repo/settings_repo'
import { recentLines } from '../pkg/log'
import { readJson, writeJson } from '../pkg/fsutil'
import type { AppPrefsVO, ConfigFileListVO, ConfigFileVO, ConfigTextVO, WorkspaceRowVO } from '../domain/types_core'

const KEY_RESTORE = 'prefs.restore_session'
const KEY_LAST_SESSION = 'prefs.last_session_file'
const KEY_LAST_ENGINE = 'prefs.last_engine_kind'
const KEY_CLOSE = 'shell.close_action'

/** 全部设置键值。 */
export function settingsAll(): Record<string, string> {
  return getAllSettings()
}

/** 合并保存设置。 */
export function settingsSave(kv: Record<string, string>): Record<string, string> {
  return saveSettings(kv)
}

/** 设当前工作区；返回工作区路径。 */
export function workspaceSet(dir: string): string {
  return setWorkspace(dir)
}

/** 忘记一个工作区。 */
export function workspaceForget(dir: string): WorkspaceRowVO[] {
  return forgetWorkspace(dir) as WorkspaceRowVO[]
}

/** 当前工作区目录；没设给空串。 */
export function currentWorkspace(): string {
  return getWorkspace()
}

/** 应用偏好总览。 */
export function prefsGet(): AppPrefsVO {
  const p = paths()
  const values: Record<string, string> = {}
  for (const [key, value] of Object.entries(getAllSettings())) {
    if (key.startsWith('ui.')) values[key] = value
  }
  const close = getSetting(KEY_CLOSE)
  return {
    values,
    auto_start: app.getLoginItemSettings().openAtLogin,
    restore_session: getSetting(KEY_RESTORE) === '1',
    last_session_file: getSetting(KEY_LAST_SESSION) ?? '',
    last_engine_kind: (getSetting(KEY_LAST_ENGINE) as 'pi' | '') ?? '',
    data_dir: p.wb_data_dir,
    log_dir: p.wb_log_dir,
    close_action: close === 'quit' ? 'quit' : 'tray',
    tray_hint_pending: false,
  }
}

/** 开机自启。 */
export function prefsSetAutoStart(enabled: boolean): AppPrefsVO {
  app.setLoginItemSettings({ openAtLogin: enabled, args: ['--hidden'] })
  return prefsGet()
}

/** 启动恢复上次会话。 */
export function prefsSetRestore(enabled: boolean): AppPrefsVO {
  saveSettings({ [KEY_RESTORE]: enabled ? '1' : '0' })
  return prefsGet()
}

/** 关窗行为。 */
export function prefsSetCloseAction(action: 'tray' | 'quit'): AppPrefsVO {
  saveSettings({ [KEY_CLOSE]: action })
  return prefsGet()
}

/** 启动时读关窗行为；给壳初始化用。 */
export function prefsCloseAction(): 'tray' | 'quit' {
  return getSetting(KEY_CLOSE) === 'quit' ? 'quit' : 'tray'
}

/** 记住最后会话，供下次启动恢复。 */
export function prefsRememberSession(engine: string, session_file: string): AppPrefsVO {
  saveSettings({ [KEY_LAST_ENGINE]: engine || 'pi', [KEY_LAST_SESSION]: session_file })
  return prefsGet()
}

/** 打开数据目录。 */
export async function prefsOpenDataDir(): Promise<void> {
  await shell.openPath(paths().wb_data_dir)
}

/** pi 的配置文件清单：agent 目录 + 工作区项目级。 */
export function configFiles(): ConfigFileListVO {
  const p = paths()
  const agent = p.wb_pi_agent_dir
  const ws = currentWorkspace()
  const entries: { file: string; label: string }[] = [
    { file: path.join(agent, 'settings.json'), label: 'pi 设置' },
    { file: path.join(agent, 'models.json'), label: '自定义模型' },
    { file: path.join(agent, 'mcp.json'), label: 'MCP 服务' },
    { file: path.join(agent, 'auth.json'), label: '凭据' },
    { file: path.join(agent, 'AGENTS.md'), label: '全局指令' },
  ]
  if (ws) {
    entries.push(
      { file: path.join(ws, '.pi', 'settings.json'), label: '项目设置' },
      { file: path.join(ws, '.pi', 'mcp.json'), label: '项目 MCP' },
      { file: path.join(ws, 'AGENTS.md'), label: '项目指令' },
    )
  }
  return {
    engine_kind: 'pi',
    entries: entries.map((e) => {
      let size = 0
      try {
        size = fs.statSync(e.file).size
      } catch {
        /* 不存在即为 0 */
      }
      return { file_path: e.file, file_label: e.label, exists: fs.existsSync(e.file), size_bytes: size }
    }),
  }
}

/** 读一个配置文件原文；太长截断。 */
export function configText(file: string): ConfigTextVO {
  const resolved = path.resolve(file)
  const allowed = configFiles().entries.some((e) => path.resolve(e.file_path) === resolved)
  if (!allowed) {
    return { file_path: file, text: '', truncated: false, found: false }
  }
  try {
    const text = fs.readFileSync(resolved, 'utf8')
    const truncated = text.length > 200_000
    return { file_path: resolved, text: truncated ? text.slice(0, 200_000) : text, truncated, found: true }
  } catch {
    return { file_path: resolved, text: '', truncated: false, found: false }
  }
}

/** 读 pi 的 JSON 配置；不存在给空对象。 */
export function readPiJson<T>(name: string): T {
  return readJson<T>(path.join(paths().wb_pi_agent_dir, name)) ?? ({} as T)
}

/** 写 pi 的 JSON 配置。 */
export function writePiJson(name: string, value: unknown): void {
  writeJson(path.join(paths().wb_pi_agent_dir, name), value)
}

/** 最近日志行。 */
export function logsRecent(): { lines: string[] } {
  return { lines: recentLines(400) }
}
