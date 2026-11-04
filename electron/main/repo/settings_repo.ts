//! 设置 KV：字符串键值；复杂结构由调用方 JSON 序列化。
import { dbConn } from './db'
import { nowMs } from '../pkg/fsutil'

/** 读一个键；没有返回 null。 */
export function getSetting(key: string): string | null {
  const row = dbConn()
    .prepare('SELECT setting_value FROM settings_kv WHERE setting_key = ?')
    .get(key) as { setting_value: string } | undefined
  return row?.setting_value ?? null
}

/** 读全部键值。 */
export function getAllSettings(): Record<string, string> {
  const rows = dbConn()
    .prepare('SELECT setting_key, setting_value FROM settings_kv')
    .all() as { setting_key: string; setting_value: string }[]
  const out: Record<string, string> = {}
  for (const row of rows) out[row.setting_key] = row.setting_value
  return out
}

/** 写一批键值（一个事务）。 */
export function saveSettings(kv: Record<string, string>): Record<string, string> {
  const stmt = dbConn().prepare(
    'INSERT INTO settings_kv (setting_key, setting_value) VALUES (?, ?) ' +
      'ON CONFLICT(setting_key) DO UPDATE SET setting_value = excluded.setting_value',
  )
  const run = dbConn().transaction((entries: [string, string][]) => {
    for (const [key, value] of entries) stmt.run(key, value)
  })
  run(Object.entries(kv))
  return getAllSettings()
}

/** 工作区历史行。 */
export interface WorkspaceRow {
  workspace_path: string
  exists: boolean
  updated_at?: string
}

const KEY_WORKSPACE = 'workspace.current'
const KEY_HISTORY = 'workspace.history'

/** 当前工作区路径；没设过给空。 */
export function getWorkspace(): string {
  return getSetting(KEY_WORKSPACE) ?? ''
}

/** 设当前工作区并记入历史。 */
export function setWorkspace(dir: string): string {
  saveSettings({ [KEY_WORKSPACE]: dir })
  const history = getWorkspaceHistory()
  const next = history.filter((row) => row.workspace_path !== dir)
  next.unshift({ workspace_path: dir, exists: true, updated_at: new Date(nowMs()).toISOString() })
  saveSettings({ [KEY_HISTORY]: JSON.stringify(next.slice(0, 20)) })
  return dir
}

/** 工作区历史（最近在前）。 */
export function getWorkspaceHistory(): WorkspaceRow[] {
  const raw = getSetting(KEY_HISTORY)
  if (!raw) return []
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as WorkspaceRow[]) : []
  } catch {
    return []
  }
}

/** 从历史里删一条；是当前目录时一并清掉当前。 */
export function forgetWorkspace(dir: string): WorkspaceRow[] {
  const next = getWorkspaceHistory().filter((row) => row.workspace_path !== dir)
  saveSettings({ [KEY_HISTORY]: JSON.stringify(next) })
  if (getWorkspace() === dir) saveSettings({ [KEY_WORKSPACE]: '' })
  return next
}
