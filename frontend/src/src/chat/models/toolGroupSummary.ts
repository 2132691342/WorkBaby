/**
 * 工具调用聚合摘要（receipt）：把一串工具调用收敛成「读取了 3 个文件 · 执行了 2 条命令」，
 * 长任务不再刷出十几行原始调用；逐条明细仍在展开区，聚合只承担「一眼看懂做了什么」。
 *
 * fail closed 原则：工具名缺失或无法归类时归入 generic 独立计数，绝不猜测合并——
 * 错误聚合比不聚合更伤信任。
 */

/** 聚合动作类型（与 i18n key tool.receipt.* 一一对应）。 */
export type ReceiptAction =
  | 'read_files'
  | 'edit_files'
  | 'run_commands'
  | 'search_web'
  | 'search_knowledge'
  | 'memory'
  | 'delegate'
  | 'generic'

export interface ReceiptPart {
  action: ReceiptAction
  count: number
}

/** 工具名 → 动作归类；未命中返回 null（fail closed，由调用方归入 generic）。 */
const ACTION_BY_TOOL: Record<string, ReceiptAction> = {
  file_read: 'read_files',
  file_list: 'read_files',
  file_grep: 'read_files',
  file_glob: 'read_files',
  doc_reader: 'read_files',
  archive_manager: 'read_files',
  file_edit: 'edit_files',
  file_write: 'edit_files',
  exec: 'run_commands',
  run_skill_script: 'run_commands',
  websearch: 'search_web',
  webfetch: 'search_web',
  http: 'search_web',
  knowledge_search: 'search_knowledge',
  memory_write: 'memory',
  delegate_task: 'delegate'
}

/** 单个工具名 → 动作；无工具名 / 未知工具返回 generic（显式归类，不猜）。 */
function actionOf(name?: string): ReceiptAction {
  if (!name) return 'generic'
  return ACTION_BY_TOOL[name] ?? 'generic'
}

/**
 * 把顺序工具调用聚合为 receipt 片段（保序：按首次出现顺序输出）。
 * 输入元素只需 name；其余字段由消费方自行处理。
 */
export function summarizeToolCalls(tools: ReadonlyArray<{ name?: string }>): ReceiptPart[] {
  const order: ReceiptAction[] = []
  const counts = new Map<ReceiptAction, number>()
  for (const tool of tools) {
    const a = actionOf(tool?.name)
    if (!counts.has(a)) {
      counts.set(a, 0)
      order.push(a)
    }
    counts.set(a, (counts.get(a) as number) + 1)
  }
  return order.map((action) => ({ action, count: counts.get(action) as number }))
}

/** 回合总耗时：已收尾工具求和 + running 工具实时补差；无任何耗时信息返回 null。 */
export function totalToolMs(
  tools: ReadonlyArray<{ durationMs?: number; startedAt?: number; state?: string }>,
  now: number
): number | null {
  let total = 0
  let seen = false
  for (const tool of tools ?? []) {
    if (tool.durationMs != null) {
      total += tool.durationMs
      seen = true
    } else if (tool.state === 'running' && tool.startedAt != null) {
      total += Math.max(0, now - tool.startedAt)
      seen = true
    }
  }
  return seen ? total : null
}

/** 毫秒 → 人话（12s / 3m 20s / 1h 5m），与 UsageBadge 的秒级口径区分（回合粒度取整）。 */
export function fmtTurnDuration(ms: number | null): string {
  if (ms == null || ms < 0) return ''
  const totalSec = Math.round(ms / 1000)
  if (totalSec < 60) return `${totalSec}s`
  const min = Math.floor(totalSec / 60)
  const sec = totalSec % 60
  if (min < 60) return sec > 0 ? `${min}m ${sec}s` : `${min}m`
  const hour = Math.floor(min / 60)
  return `${hour}h ${min % 60}m`
}
