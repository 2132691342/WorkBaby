import type { Ref } from 'vue'
import { ref } from 'vue'

/**
 * 全局命令面板：状态 + 动作注册中心。
 *
 * <p>单例 module state（非 Pinia——只有开合与注册表，无持久化诉求）。
 * 命令项由消费方注册（CommandPalette.vue 启动时注册导航与全局动作），
 * 后续可在此挂 Skill / 工作流等深层命令，替换为 vue-command-palette 等库时
 * 只需迁移本文件与渲染层，命令注册协议不变。
 */

/** 命令项（title 由注册方翻译好；本模块不做 i18n）。 */
export interface CommandItem {
  id: string
  title: string
  group: 'nav' | 'action' | 'session' | 'mode' | 'tool'
  hint?: string
  /** 图标组件（接受 lucide 自绘 / EP 图标 / 任何 Vue 组件；any 是 Vue <component :is> binding 的最小公约数）。 */
  icon?: any
  run: () => void
}

const open = ref(false)
const registry = new Map<string, CommandItem>()

/** 命令面板是否打开。 */
export function paletteOpen(): Ref<boolean> {
  return open
}

/** 打开面板。 */
export function openPalette(): void {
  open.value = true
}

/** 关闭面板。 */
export function closePalette(): void {
  open.value = false
}

/** 注册命令（同 id 覆盖）。 */
export function registerCommands(items: CommandItem[]): void {
  for (const it of items) registry.set(it.id, it)
}

/** 全量命令（注册顺序稳定）。 */
export function listCommands(): CommandItem[] {
  return [...registry.values()]
}

/**
 * 子序列模糊匹配（纯函数，可单测）：
 * query 的每个字符按序出现在 text 中即命中；大小写不敏感；空 query 全通过。
 */
export function fuzzyMatch(query: string, text: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  const t = text.toLowerCase()
  let i = 0
  for (const ch of t) {
    if (ch === q[i]) i++
    if (i >= q.length) return true
  }
  return i >= q.length
}

/** 过滤命令（title + hint 双字段匹配，纯函数）。 */
export function filterCommands(items: CommandItem[], query: string): CommandItem[] {
  if (!query.trim()) return items
  return items.filter((it) => fuzzyMatch(query, it.title) || (it.hint ? fuzzyMatch(query, it.hint) : false))
}
