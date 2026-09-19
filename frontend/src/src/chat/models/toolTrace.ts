/**
 * 工具调用 → 「动词叙事」迹线模型。
 *
 * <p>一行工具迹线 = 动词（查阅/编辑/终端/委派…）+ 目标（文件名粗体 + 目录淡色 / 命令 / 查询词）
 * + 增删徽标（+N -N，仅文件改动）。与 toolVisuals 的图标映射互补：那套回答「这是什么工具」，
 * 这套回答「它在对什么东西做什么」——目标从 args 结构化提取，不是把 JSON 原文截断。
 */
import type { Component } from 'vue'
import { Search, Pencil, FilePlus2, Terminal, Bot, Brain, GitBranch, BookOpen, ListTree, MessageSquare, Globe } from '@/components/common/icons'

/** 迹线动词类别（i18n 键 tool.verb.<kind>）。 */
export type TraceKind =
  | 'read' // 查阅：读文件 / 列目录 / 检索
  | 'edit' // 编辑：改文件
  | 'write' // 写入：新建文件
  | 'exec' // 终端：跑命令 / 跑脚本
  | 'search' // 检索：联网
  | 'knowledge' // 知识库
  | 'memory' // 记忆
  | 'workflow' // 工作流
  | 'delegate' // 子代理委派
  | 'plan' // 待办 / 计划
  | 'ask' // 向用户提问
  | 'other'

const KIND_BY_TOOL: Record<string, TraceKind> = {
  file_read: 'read',
  file_list: 'read',
  file_glob: 'read',
  file_grep: 'read',
  doc_reader: 'read',
  archive_manager: 'read',
  file_edit: 'edit',
  file_write: 'write',
  exec: 'exec',
  run_skill_script: 'exec',
  websearch: 'search',
  webfetch: 'search',
  http: 'search',
  knowledge_search: 'knowledge',
  memory_write: 'memory',
  run_workflow: 'workflow',
  delegate_task: 'delegate',
  todo: 'plan',
  request_input: 'ask'
}

const ICON_BY_KIND: Record<TraceKind, Component> = {
  read: Search,
  edit: Pencil,
  write: FilePlus2,
  exec: Terminal,
  search: Globe,
  knowledge: BookOpen,
  memory: Brain,
  workflow: GitBranch,
  delegate: Bot,
  plan: ListTree,
  ask: MessageSquare,
  other: Search
}

/** 工具名 → 迹线类别；未知工具归 other（不猜语义）。 */
export function traceKind(name?: string): TraceKind {
  if (!name) return 'other'
  return KIND_BY_TOOL[name] ?? 'other'
}

/** 迹线类别 → 图标。 */
export function traceIcon(name?: string): Component {
  return ICON_BY_KIND[traceKind(name)]
}

/** 迹线目标：main 是一眼看到的东西（文件名 / 命令首段 / 查询词），sub 是弱化的补充（目录 / 全路径）。 */
export interface TraceTarget {
  main: string
  sub?: string
}

/** args JSON 里优先取哪个字段当目标（与后端工具参数命名对齐）。 */
const TARGET_KEYS = ['path', 'file_path', 'command', 'url', 'query', 'pattern', 'name', 'task', 'input']

/**
 * 从工具参数提取迹线目标。
 *
 * <p>文件类参数拆「文件名 + 目录」两段展示；命令只留一行；
 * 其余键取第一个非空字符串。解析失败返回 null（头部退化为只显示动词）。
 */
export function traceTarget(argsJson: string | undefined): TraceTarget | null {
  if (!argsJson) return null
  let o: Record<string, unknown>
  try {
    o = JSON.parse(argsJson) as Record<string, unknown>
  } catch {
    const one = oneLine(argsJson, 64)
    return one ? { main: one } : null
  }
  for (const k of TARGET_KEYS) {
    const v = o[k]
    if (typeof v !== 'string' || !v.trim()) continue
    const one = oneLine(v, 96)
    if (!one) continue
    // 文件路径 → 文件名为主、目录为辅（「app.go internal/bootstrap/」形态）
    // Windows 反斜杠与 POSIX 斜杠都拆；含空格的路径不拆（拆了反而难读）
    if (k === 'path' || k === 'file_path') {
      const sep = Math.max(one.lastIndexOf('/'), one.lastIndexOf('\\'))
      if (sep > 0 && !one.includes(' ')) {
        return { main: one.slice(sep + 1), sub: one.slice(0, sep + 1) }
      }
    }
    if (k === 'command') return { main: one }
    return { main: one }
  }
  return null
}

/** 委派目标：args.agent（子代理名）；空回退 default。 */
export function traceDelegateAgent(argsJson: string | undefined): string {
  if (!argsJson) return 'default'
  try {
    const o = JSON.parse(argsJson) as Record<string, unknown>
    const a = o.agent
    return typeof a === 'string' && a.trim() ? a.trim() : 'default'
  } catch {
    return 'default'
  }
}

function oneLine(s: string, max: number): string {
  const one = s.replace(/\s+/g, ' ').trim()
  return one.length > max ? `${one.slice(0, max)}…` : one
}

/** diff 统计：+ / - 行数（hunk 头与文件头不计）。 */
export function countDiffLines(diff: string | undefined): { added: number; removed: number } {
  if (!diff) return { added: 0, removed: 0 }
  let added = 0
  let removed = 0
  for (const raw of diff.split('\n')) {
    if (raw.startsWith('+++') || raw.startsWith('---') || raw.startsWith('@@')) continue
    if (raw.startsWith('+')) added++
    else if (raw.startsWith('-')) removed++
  }
  return { added, removed }
}
