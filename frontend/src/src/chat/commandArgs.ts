/**
 * 自定义命令的参数展开。
 *
 * 约定：命令体就是发送给模型的内容，`/命令名 参数…` 里的参数按占位符注入。
 * - `$ARGUMENTS` → 全部参数（原文，保留引号内的空格）
 * - `$1`..`$9` → 第 n 个位置参数（支持 "带空格的参数" 与 '单引号'）
 *
 * 展开顺序：先替换位置参数、再替换 `$ARGUMENTS`。反过来会让参数正文里的 `$1`
 * 被当成占位符二次展开（用户参数被静默吃掉）。
 */

/** 命令模板的最小结构（兼容 store 的 SlashCommand 与面板的本地类型）。 */
export interface CommandTemplate {
  name: string
  prompt?: string
}

/** tokenizeArgs 按 shell 习惯切分参数：双引号 / 单引号内的空格保留。 */
export function tokenizeArgs(args: string): string[] {
  const out: string[] = []
  const re = /"([^"]*)"|'([^']*)'|(\S+)/g
  let m: RegExpExecArray | null
  while ((m = re.exec(args)) !== null) {
    out.push(m[1] ?? m[2] ?? m[3] ?? '')
  }
  return out
}

/** expandCommandPrompt 展开模板占位符；`$ARGUMENTS` 为空串时按空串注入。 */
export function expandCommandPrompt(template: string, args: string): string {
  const trimmed = args.trim()
  const tokens = tokenizeArgs(trimmed)
  return template
    .replace(/\$([1-9])/g, (_all, d: string) => tokens[Number(d) - 1] ?? '')
    .replace(/\$ARGUMENTS/g, trimmed)
}

/**
 * expandSlashDraft 把草稿里的 `/命令名 参数…` 展开成最终发送内容。
 * 不是带模板的自定义命令（内置命令 / 普通文本 / 未命中命令名）时原样返回。
 */
export function expandSlashDraft(draft: string, commands: readonly CommandTemplate[]): string {
  const m = /^\/([A-Za-z0-9_-]+)(?:\s+([\s\S]*))?$/.exec(draft.trim())
  if (!m) return draft
  const hit = commands.find((c) => c.name === m[1] && (c.prompt ?? '') !== '')
  if (!hit?.prompt) return draft
  return expandCommandPrompt(hit.prompt, m[2] ?? '')
}
