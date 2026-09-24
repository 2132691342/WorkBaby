/**
 * 消息块纯函数模型层。
 *
 * 后端 message_blocks 表 → 前端消息分块渲染的唯一归一化出口：
 * 历史复现（message.blocks）与流式态（streaming 临时结构）共用同一套块形状，
 * MessageList 组件只做渲染，不解析 payload。
 */
import type { Message, MessageBlock, MessageBlockKind, SkillHit } from '../../types/api'
import type { ToolCallInfo } from '../../stores/chat/ChatStreamDecoder'

/** 归一化后的块形状（组件渲染直接消费）。 */
export interface ResolvedBlock {
  kind: MessageBlockKind
  seq: number
  /** tool_call / tool_result / artifact / genui 的 JSON 解析结果；thinking 为纯文本。 */
  data: Record<string, unknown> | null
  /** thinking 块的文本内容。 */
  text: string
}

/** 工具调用块 payload（chat:tool / BlockToolCall）。 */
export interface ToolCallBlockData {
  id: string
  name: string
  arguments?: string
}

/** 工具结果块 payload（chat:tool-result / BlockToolResult）。 */
export interface ToolResultBlockData {
  tool_call_id: string
  name: string
  content?: string
  error?: string
  duration_ms?: number
  /** 审批拒绝（Refused 语义）：非故障，前端展示「已拒绝」而非错误态。 */
  refused?: boolean
  /**
   * 工具元数据：cwd / same_failure_count / adaptive_hint / truncated_bytes / duration_ms 等。
   * 后端 chat:tool-result 事件载荷里的 meta map 直接透传（exec 落 cwd，其余按需）。
   * 前端按 key 取值渲染：cwd 在工具卡标题旁显示，same_failure_count 作失败计数徽标，
   * adaptive_hint="1" 时把 [guard] 段从 content 中剥离并单独高亮。
   */
  meta?: Record<string, string>
}

/** 安全解析块 payload JSON；失败返回 null（脏数据不拖垮整条消息渲染）。 */
function parsePayload(block: MessageBlock): Record<string, unknown> | null {
  try {
    const v = JSON.parse(block.payload)
    return v && typeof v === 'object' ? (v as Record<string, unknown>) : null
  } catch {
    return null
  }
}

/** 单块归一化。 */
function resolveBlock(block: MessageBlock): ResolvedBlock {
  if (block.kind === 'thinking') {
    return { kind: block.kind, seq: block.seq, data: null, text: block.payload }
  }
  return { kind: block.kind, seq: block.seq, data: parsePayload(block), text: '' }
}

/**
 * 消息 → 渲染块序列（按 seq 升序；空/脏块过滤）。
 *
 * 优先取持久化 blocks；无块且消息带 tool_calls_json 时降级从 tool_calls_json 重建
 * tool_call 块（兼容无块的历史消息，至少能看到调用过什么）。
 */
export function resolveMessageBlocks(msg: Message): ResolvedBlock[] {
  if (msg.blocks && msg.blocks.length > 0) {
    return msg.blocks
      .slice()
      .sort((a, b) => a.seq - b.seq)
      .map(resolveBlock)
  }
  // 历史消息降级：assistant.tool_calls_json → tool_call 块（无结果信息）
  if (msg.role === 'assistant' && msg.tool_calls_json) {
    try {
      const calls = JSON.parse(msg.tool_calls_json) as Array<{
        id?: string
        function?: { name?: string; arguments?: string }
      }>
      if (Array.isArray(calls)) {
        return calls.map((c, i) => ({
          kind: 'tool_call' as MessageBlockKind,
          seq: i,
          data: {
            id: c.id ?? String(i),
            name: c.function?.name ?? 'unknown',
            arguments: c.function?.arguments ?? ''
          },
          text: ''
        }))
      }
    } catch {
      /* 脏 JSON 忽略 */
    }
  }
  return []
}

/** 渲染分组：连续工具块归入同一组（渲染成一张「过程卡」），其余块各自一组。 */
export interface BlockGroup<T> {
  /** 稳定的 v-for key。 */
  key: string
  /** 连续工具块（长度 ≥1）；为空的组表示这是一个独立块。 */
  tools: T[]
  /** 独立块（tools 为空时有效）。 */
  one: T | null
}

/**
 * 按**相邻性**把块序列切成渲染分组，不重排任何元素。
 *
 * <p>正文块会自然切断分组，所以「叙述 → 工具组 → 叙述」的顺序被完整保留。
 * 顺序由上游的 seq 决定，这里只做切分——任何"按类型归类"的写法都会打乱消息顺序。
 */
export function groupToolRuns<T extends { kind: string; seq: number }>(units: T[]): BlockGroup<T>[] {
  const out: BlockGroup<T>[] = []
  for (const u of units) {
    const isTool = u.kind === 'tool_call' || u.kind === 'tool_result'
    const last = out[out.length - 1]
    if (isTool && last && last.tools.length > 0) {
      last.tools.push(u)
      continue
    }
    if (isTool) {
      out.push({ key: `tools-${u.seq}`, tools: [u], one: null })
    } else {
      out.push({ key: `${u.kind}-${u.seq}`, tools: [], one: u })
    }
  }
  return out
}

/** 块是否为「带错误结果」的 tool_result（refused 不算错误）。 */
export function isFailedResultBlock(block: ResolvedBlock): boolean {
  if (block.kind !== 'tool_result') return false
  const d = block.data as Partial<ToolResultBlockData> | null
  return Boolean(d?.error)
}

/**
 * 块序列 → 本轮命中的 Skill（skill 块可能为空 name 的脏数据，过滤掉）。
 *
 * 技能命中发生在首帧之前，取 seq 最小的那一条即可（一轮最多一次命中）。
 */
export function skillHitOfBlocks(blocks: ResolvedBlock[]): SkillHit | null {
  for (const b of blocks) {
    if (b.kind !== 'skill' || !b.data) continue
    const d = b.data as Partial<SkillHit>
    if (!d.name) continue
    return {
      name: d.name,
      source: d.source,
      version: d.version,
      description: d.description,
      tools: d.tools ?? [],
      injected_chars: d.injected_chars
    }
  }
  return null
}

/** 消息 → 本轮命中的 Skill（历史回看用；无 skill 块返回 null）。 */
export function messageSkillHit(msg: Message): SkillHit | null {
  return skillHitOfBlocks(resolveMessageBlocks(msg))
}

/** 块是否为审批拒绝结果。 */
export function isRefusedResultBlock(block: ResolvedBlock): boolean {
  if (block.kind !== 'tool_result') return false
  const d = block.data as Partial<ToolResultBlockData> | null
  return Boolean(d?.refused)
}

/** 终止原因 → 展示级别（与后端 domain.StopReasonSeverity 契约一致）。 */
export type StopSeverity = 'info' | 'warning' | 'error'

/**
 * 可恢复终态集合（唯一权威，StopReasonBanner / MessageList 共用）：
 * 这些终态意味着「任务没做完，但可以接着做」，展示「继续」按钮。
 * 两处判断不一致会导致「文案说可续跑、按钮不出现」的断裂。
 */
export const RESUMABLE_STOP_REASONS = new Set([
  'cancelled',
  'tool_error_limit',
  'max_turns',
  'token_budget',
  'interrupted'
])

const WARNING_REASONS = new Set([
  'cancelled',
  'max_turns',
  'tool_error_limit',
  'token_budget',
  'stagnation',
  'interrupted'
])

export function stopReasonSeverity(reason: string | null | undefined): StopSeverity {
  if (!reason) return 'info'
  if (reason === 'completed') return 'info'
  if (WARNING_REASONS.has(reason)) return 'warning'
  return 'error'
}

/**
 * 审批拒绝的协议文本常量：refused 块的展示文案与旧数据判定共用单一定义，
 * 避免散落的中文字符串字面量（改一处漏一处）。
 */
export const REFUSED_RESULT_TEXT = '已拒绝：用户未批准该工具调用'

/**
 * 工具结果文本协议常量（仅用于回读历史数据）。
 *
 * 历史消息把工具结果落成带前缀的纯文本，渲染与判定必须与当时的写法一致；
 * 这些串是协议而非文案，集中在此定义，禁止散落到组件里做字面量判断
 * （改文案时不会连带改坏协议解析）。
 */
export const RESULT_REFUSED_PREFIX = '已拒绝'
export const RESULT_ERROR_PREFIX = 'error: '
/** 剥掉整行 error 头（保留后续正文）。 */
export const RESULT_ERROR_LINE_RE = /^error: [^\n]*\n?/

/** 终止原因 → i18n key（字典 stop.reason.*）；未识别返回空串（消费方给兜底文案）。 */
export function stopReasonText(reason: string | null | undefined): string {
  switch (reason) {
    case 'completed':
      return 'stop.reason.completed'
    case 'cancelled':
      return 'stop.reason.cancelled'
    case 'max_turns':
      return 'stop.reason.max_turns'
    case 'tool_error_limit':
      return 'stop.reason.tool_error_limit'
    case 'token_budget':
      return 'stop.reason.token_budget'
    case 'stagnation':
      return 'stop.reason.stagnation'
    case 'interrupted':
      return 'stop.reason.interrupted'
    case 'error':
      return 'stop.reason.error'
    default:
      return ''
  }
}

// ===== diff 识别与解析=====

/** diff 行类型。 */
export interface DiffLine {
  type: 'add' | 'del' | 'hunk' | 'ctx'
  text: string
}

/**
 * 剥离 content 中的 <think> 块。
 *
 * <p>部分 OpenAI 兼容端点（DeepSeek 风格）把推理写进正文用 <think>…</think> 包裹，
 * 而非独立 reasoning 字段——后端 thinking 分离只覆盖后者，这里在渲染层兜底：
 * 闭合块全局移除（跨行），流式中未闭合的尾部 <think>… 前缀一并移除（避免闪现原始标签）。
 * 思考内容另有原生 thinking 通道展示，此处剥离不损失可读信息。
 */
export function stripThinkBlocks(content: string): string {
  if (!content.includes('<think>')) return content
  let out = content.replace(/<think>[\s\S]*?<\/think>/g, '')
  out = out.replace(/<think>[\s\S]*$/, '')
  return out.trim()
}

/**
 * 结果内容是否为 unified diff。
 * 信号（满足其一）：`diff --git` 头、`@@ -x +y @@` hunk 头、`+++ /---` 文件头对。
 * 启发式按 rune 前缀判定，误判面（markdown `---` 分割线 + `+++` 极少共存）可接受。
 */
export function looksLikeDiff(content: string): boolean {
  const s = content.slice(0, 4000)
  if (s.includes('diff --git ')) return true
  if (/^@@ -\d+(,\d+)? \+\d+(,\d+)? @@/m.test(s)) return true
  const plus = (s.match(/^\+\+\+ /gm) ?? []).length
  const minus = (s.match(/^--- /gm) ?? []).length
  return plus > 0 && minus > 0
}

/** unified diff → 着色行序列（`+++ /--- ` 开头是文件头不算增删行）。 */
export function parseDiffLines(content: string): DiffLine[] {
  const lines = content.split('\n')
  if (lines.length > 0 && lines[lines.length - 1] === '') lines.pop()
  return lines.map((text) => {
    if (text.startsWith('@@')) return { type: 'hunk' as const, text }
    if (text.startsWith('+++ ') || text.startsWith('--- ')) return { type: 'ctx' as const, text }
    if (text.startsWith('+')) return { type: 'add' as const, text }
    if (text.startsWith('-')) return { type: 'del' as const, text }
    return { type: 'ctx' as const, text }
  })
}

/** diff 行 → 配色 class（wb token；组件直接绑定）。 */
export function diffLineClass(type: DiffLine['type']): string {
  switch (type) {
    case 'add':
      return 'bg-wb-mint/10 text-wb-mint'
    case 'del':
      return 'bg-wb-danger/10 text-wb-danger'
    case 'hunk':
      return 'text-wb-primary-strong'
    default:
      return 'text-wb-muted'
  }
}

/** 带行号的 diff 行（DiffView 渲染用）：hunk 头解析出两侧起始行号后逐行推进。 */
export interface DiffRow {
  type: DiffLine['type']
  text: string
  /** 旧文件行号（del / ctx 有值；hunk 头与文件头为 null）。 */
  oldNo: number | null
  /** 新文件行号（add / ctx 有值）。 */
  newNo: number | null
}

/**
 * unified diff → 带行号行序列。
 *
 * <p>行号从 `@@ -a,b +c,d @@` hunk 头取种子逐行推进；无 hunk 头的非标准 diff
 * （模型手写的简化 +/- 块）退化为只标新侧序号，保证不显示错位行号。
 * `+++/---` 文件头行保留但无行号。
 */
export function parseDiffRows(content: string): DiffRow[] {
  const lines = content.split('\n')
  if (lines.length > 0 && lines[lines.length - 1] === '') lines.pop()
  const out: DiffRow[] = []
  let oldNo = 0
  let newNo = 0
  let sawHunk = false
  let newOnlySeq = 0
  for (const text of lines) {
    if (text.startsWith('@@')) {
      const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(text)
      if (m) {
        oldNo = Number(m[1])
        newNo = Number(m[2])
        sawHunk = true
      }
      out.push({ type: 'hunk', text, oldNo: null, newNo: null })
      continue
    }
    if (text.startsWith('+++ ') || text.startsWith('--- ')) {
      out.push({ type: 'ctx', text, oldNo: null, newNo: null })
      continue
    }
    if (text.startsWith('+')) {
      newOnlySeq = sawHunk ? newNo : newOnlySeq + 1
      out.push({ type: 'add', text, oldNo: null, newNo: newOnlySeq })
      if (sawHunk) newNo++
      continue
    }
    if (text.startsWith('-')) {
      out.push({ type: 'del', text, oldNo: sawHunk ? oldNo : null, newNo: null })
      if (sawHunk) oldNo++
      continue
    }
    out.push({ type: 'ctx', text, oldNo: sawHunk ? oldNo : null, newNo: sawHunk ? newNo : ++newOnlySeq })
    if (sawHunk) {
      oldNo++
      newNo++
    } else {
      newOnlySeq++
    }
  }
  return out
}

/** 块序列 → ToolCallInfo[]（配对 tool_call/tool_result，复用 TaskTimeline 渲染历史过程）。 */
export function blocksToToolCalls(blocks: ResolvedBlock[]): ToolCallInfo[] {
  const calls = new Map<string, ToolCallInfo>()
  for (const b of blocks) {
    if (b.kind === 'tool_call' && b.data) {
      const d = b.data as Partial<ToolCallBlockData>
      calls.set(d.id ?? '', {
        id: d.id ?? '',
        name: d.name ?? 'unknown',
        args: d.arguments,
        state: 'running'
      })
    } else if (b.kind === 'tool_result' && b.data) {
      const d = b.data as Partial<ToolResultBlockData>
      const text = d.error ? `${RESULT_ERROR_PREFIX}${d.error}\n${d.content ?? ''}` : (d.content ?? '')
      const hit = calls.get(d.tool_call_id ?? '')
      if (hit) {
        hit.result = d.refused ? REFUSED_RESULT_TEXT : text
        hit.state = d.error && !d.refused ? 'error' : 'success'
        hit.duration_ms = d.duration_ms
      } else {
        calls.set(d.tool_call_id ?? '', {
          id: d.tool_call_id ?? '',
          name: d.name ?? 'unknown',
          result: d.refused ? REFUSED_RESULT_TEXT : text,
          state: d.error && !d.refused ? 'error' : 'success',
          duration_ms: d.duration_ms
        })
      }
    }
  }
  return [...calls.values()]
}
