//! 工作区文件浏览：目录清单、按名搜索与文本读取；路径一律限制在工作区内。
import fs from 'node:fs'
import path from 'node:path'
import { currentWorkspace } from './settings_service'
import { appError } from '../pkg/apperror'
import { CODE_NOT_FOUND, CODE_PATH_ESCAPE } from '../domain/errors'
import type { FileEntryVO, FileListVO, FileReadVO, FileSearchVO } from '../domain/types_data'

const READ_MAX = 1024 * 1024
const SEARCH_LIMIT = 50
const SKIP_DIRS = new Set(['node_modules', '.git', 'dist', '.pi', '.venv', '__pycache__'])

/** 解析工作区内的相对/绝对路径；越界直接拒。 */
function resolveInside(input: string): string {
  const ws = currentWorkspace()
  if (!ws) throw appError(CODE_PATH_ESCAPE, '请先选择工作区')
  const resolved = path.resolve(input.startsWith('/') || input.startsWith('\\') || /^[A-Za-z]:/.test(input) ? input : path.join(ws, input))
  const root = path.resolve(ws)
  if (resolved !== root && !resolved.startsWith(root + path.sep)) {
    throw appError(CODE_PATH_ESCAPE, '路径越出工作区', resolved)
  }
  return resolved
}

/** 目录清单：目录在前、按名排序，最多 500 条。 */
export function fileList(input: string): FileListVO {
  const dir = input ? resolveInside(input) : path.resolve(currentWorkspace())
  let names: string[] = []
  try {
    names = fs.readdirSync(dir)
  } catch {
    throw appError(CODE_NOT_FOUND, `目录不存在：${input || currentWorkspace()}`)
  }
  const entries: FileEntryVO[] = []
  for (const name of names) {
    if (name === '.git') continue
    const full = path.join(dir, name)
    let isDir = false
    let size: number | undefined
    try {
      isDir = fs.statSync(full).isDirectory()
      if (!isDir) size = fs.statSync(full).size
    } catch {
      continue
    }
    entries.push({ entry_name: name, dir: isDir, size })
  }
  entries.sort((a, b) => (a.dir === b.dir ? a.entry_name.localeCompare(b.entry_name) : a.dir ? -1 : 1))
  const truncated = entries.length > 500
  return { dir_path: dir, entries: entries.slice(0, 500), truncated }
}

/** 按文件名递归搜索；跳过依赖与构建目录，深度 6。 */
export function fileSearch(query: string, limit = SEARCH_LIMIT): FileSearchVO {
  const ws = currentWorkspace()
  if (!ws || !query.trim()) return { query, hits: [], truncated: false }
  const needle = query.toLowerCase()
  const hits: FileHitAcc[] = []
  walk(ws, ws, needle, hits, 0)
  const truncated = hits.length > limit
  return {
    query,
    hits: hits.slice(0, limit).map((h) => ({ file_path: h.file_path, entry_name: h.entry_name })),
    truncated,
  }
}

interface FileHitAcc {
  file_path: string
  entry_name: string
}

function walk(root: string, dir: string, needle: string, hits: FileHitAcc[], depth: number): void {
  if (depth > 6 || hits.length > SEARCH_LIMIT * 2) return
  let names: string[] = []
  try {
    names = fs.readdirSync(dir)
  } catch {
    return
  }
  for (const name of names) {
    const full = path.join(dir, name)
    let stat: fs.Stats
    try {
      stat = fs.statSync(full)
    } catch {
      continue
    }
    if (stat.isDirectory()) {
      if (!SKIP_DIRS.has(name)) walk(root, full, needle, hits, depth + 1)
      continue
    }
    if (name.toLowerCase().includes(needle)) {
      hits.push({ file_path: full, entry_name: name })
    }
  }
}

/** 读文本文件；二进制返回占位说明，超限截断。 */
export function fileRead(input: string): FileReadVO {
  const file = resolveInside(input)
  let stat: fs.Stats
  try {
    stat = fs.statSync(file)
  } catch {
    throw appError(CODE_NOT_FOUND, `文件不存在：${input}`)
  }
  if (stat.isDirectory()) throw appError(CODE_NOT_FOUND, '这是一个目录，不是文件')
  const buf = fs.readFileSync(file)
  let binary = false
  for (let i = 0; i < Math.min(buf.length, 8000); i += 1) {
    if (buf[i] === 0) {
      binary = true
      break
    }
  }
  const truncated = buf.length > READ_MAX
  const content = binary ? `（二进制文件，${buf.length} 字节）` : buf.subarray(0, READ_MAX).toString('utf8')
  return { file_path: file, content, size: buf.length, binary, truncated }
}

/** 把绝对路径限制校验暴露给其他服务复用。 */
export function safeWorkspacePath(input: string): string {
  return resolveInside(input)
}
