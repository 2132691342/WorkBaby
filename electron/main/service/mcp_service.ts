//! MCP 管理：pi 的 mcp.json（用户级）与工作区 .pi/mcp.json（项目级）的增删改查与开关。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import { readJson, writeJson } from '../pkg/fsutil'
import { splitCommandLine } from '../pkg/cmdline'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_MCP_READ, CODE_MCP_WRITE } from '../domain/errors'
import { currentWorkspace } from './settings_service'
import type { McpListDataVO, McpSaveREQ, McpServerVO } from '../domain/types_data'

/** pi mcp.json 的条目形状（只取我们关心的字段）。 */
interface McpEntry {
  command?: string
  args?: string[]
  url?: string
  env?: Record<string, string>
  headers?: Record<string, string>
  description?: string
  enabled?: boolean
  cwd?: string
  timeout?: number
  [key: string]: unknown
}

type McpFile = { mcpServers: Record<string, McpEntry> }

function userPath(): string {
  return path.join(paths().wb_pi_agent_dir, 'mcp.json')
}

function projectPath(): string | null {
  const ws = currentWorkspace()
  return ws ? path.join(ws, '.pi', 'mcp.json') : null
}

function readFile(file: string): McpFile {
  const raw = readJson<McpFile>(file)
  if (!raw) return { mcpServers: {} }
  return { mcpServers: raw.mcpServers ?? {} }
}

function writeFile(file: string, data: McpFile): void {
  writeJson(file, data)
}

/** 清单：用户级 + 项目级合并，项目级同名覆盖用户级。 */
export function mcpList(_live: boolean): McpListDataVO {
  const servers: McpServerVO[] = []
  const errors: string[] = []
  const u_path = userPath()
  const p_path = projectPath()
  let user: McpFile = { mcpServers: {} }
  try {
    user = readFile(u_path)
  } catch (err) {
    errors.push(`用户级 mcp.json 读取失败：${rawMessage(err)}`)
  }
  let project: McpFile = { mcpServers: {} }
  if (p_path && fs.existsSync(p_path)) {
    try {
      project = readFile(p_path)
    } catch (err) {
      errors.push(`项目级 mcp.json 读取失败：${rawMessage(err)}`)
    }
  }
  for (const [name, entry] of Object.entries(user.mcpServers)) {
    if (name in project.mcpServers) continue
    servers.push(toRow(name, entry, 'user', u_path))
  }
  for (const [name, entry] of Object.entries(project.mcpServers)) {
    servers.push(toRow(name, entry, 'project', p_path as string))
  }
  return {
    engine_kind: 'pi',
    servers,
    user_path: u_path,
    project_path: p_path ?? undefined,
    read_errors: errors.length ? errors : undefined,
    live_checked: false,
  }
}

function toRow(name: string, entry: McpEntry, scope: 'user' | 'project', file: string): McpServerVO {
  const url = typeof entry.url === 'string' ? entry.url : undefined
  const command = typeof entry.command === 'string' ? entry.command : undefined
  return {
    engine_kind: 'pi',
    server_name: name,
    transport_kind: url ? 'http' : 'stdio',
    command_line: command ? [command, ...(entry.args ?? [])].join(' ') : undefined,
    server_url: url,
    desc: typeof entry.description === 'string' ? entry.description : undefined,
    enabled: entry.enabled !== false,
    disabled_by_deny: false,
    scope_kind: scope,
    config_path: file,
  }
}

/** 保存：写用户级文件；server_url 有值即 HTTP，否则拆 command_text。 */
export function mcpSave(req: McpSaveREQ): McpListDataVO {
  const name = req.server_name.trim()
  if (!name) throw appError(CODE_MCP_WRITE, '服务名不能为空')
  const file = userPath()
  const data = readFile(file)
  const entry: McpEntry = {}
  if (req.server_url?.trim()) {
    entry.url = req.server_url.trim()
    if (req.transport_kind === 'stdio') throw appError(CODE_MCP_WRITE, 'URL 服务不能配置命令行')
  } else if (req.command_text?.trim()) {
    const parts = splitCommandLine(req.command_text.trim())
    if (parts.length === 0) throw appError(CODE_MCP_WRITE, '命令行不能为空')
    entry.command = parts[0]
    if (parts.length > 1) entry.args = parts.slice(1)
  } else {
    throw appError(CODE_MCP_WRITE, '命令行与 URL 至少填一个')
  }
  if (req.desc?.trim()) entry.description = req.desc.trim()
  if (req.env_list?.length) {
    const env: Record<string, string> = {}
    for (const line of req.env_list) {
      const idx = line.indexOf('=')
      if (idx > 0) env[line.slice(0, idx).trim()] = line.slice(idx + 1)
    }
    if (Object.keys(env).length > 0) entry.env = env
  }
  const prev = data.mcpServers[name]
  if (prev && prev.enabled === false) entry.enabled = false
  data.mcpServers[name] = entry
  try {
    writeFile(file, data)
  } catch (err) {
    throw wrapError(CODE_MCP_WRITE, 'mcp.json 写入失败', err)
  }
  return mcpList(false)
}

/** 开关：找到条目改 enabled；项目级同名条目优先。 */
export function mcpToggle(server_name: string, enabled: boolean): McpListDataVO {
  const targets: (string | null)[] = [projectPath(), userPath()]
  for (const file of targets) {
    if (!file || !fs.existsSync(file)) continue
    const data = readFile(file)
    if (server_name in data.mcpServers) {
      data.mcpServers[server_name].enabled = enabled
      try {
        writeFile(file, data)
      } catch (err) {
        throw wrapError(CODE_MCP_WRITE, 'mcp.json 写入失败', err)
      }
      return mcpList(false)
    }
  }
  throw appError(CODE_MCP_WRITE, `未找到 MCP 服务：${server_name}`)
}

/** 删除：用户级与项目级都尝试移除。 */
export function mcpRemove(server_name: string): McpListDataVO {
  for (const file of [projectPath(), userPath()]) {
    if (!file || !fs.existsSync(file)) continue
    const data = readFile(file)
    if (server_name in data.mcpServers) {
      delete data.mcpServers[server_name]
      try {
        writeFile(file, data)
      } catch (err) {
        throw wrapError(CODE_MCP_WRITE, 'mcp.json 写入失败', err)
      }
    }
  }
  return mcpList(false)
}

/** 跨引擎复制：目标只有 pi，原样返回清单。 */
export function mcpCopy(server_name: string, enabled: boolean): McpListDataVO {
  void server_name
  void enabled
  return mcpList(false)
}

/** MCP 服务总数（健康面板用）。 */
export function mcpCount(): number {
  return mcpList(false).servers.length
}

function rawMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}
