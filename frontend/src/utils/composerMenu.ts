// 输入区的斜杠菜单命令表与待发送状态：菜单组件、输入区、命令执行三处共用。
// 抽到组件外只有一条理由——<script setup> 不能导出常量，两边各抄一份会漂移。

/** 内置命令码。 */
export type BuiltinKey =
  | 'new'
  | 'model'
  | 'thinking'
  | 'compact'
  | 'session'
  | 'export'
  | 'copy'
  | 'reload'
  | 'login'
  | 'settings'

/** 菜单里的一条候选项。 */
export interface SlashItem {
  /** 内置命令还是技能 */
  kind: 'cmd' | 'skill'
  /** 命令码或技能名 */
  key: string
  /** 主行文案 */
  name: string
  /** 副行文案 */
  desc: string
}

/** 待发送的图片：粘贴或选择进来的图。 */
export interface PendingImage {
  /** 生成 id 用的单调序号（渲染 key 与删除都用它） */
  id: number
  /** 展示名 */
  name: string
  /** MIME 类型 */
  mime: string
  /** 纯 base64（无 data: 前缀） */
  base64: string
  /** 图片自然宽度（0 = 还没解码完） */
  width: number
  /** 图片自然高度 */
  height: number
}

/** 内置命令表：文案键交给 i18n，界面语言一换整表跟着换。 */
export const COMMANDS: { key: BuiltinKey; nameKey: string; descKey: string }[] = [
  { key: 'new', nameKey: 'chat.slashNew', descKey: 'chat.slashNewDesc' },
  { key: 'model', nameKey: 'chat.slashModel', descKey: 'chat.slashModelDesc' },
  { key: 'thinking', nameKey: 'chat.slashThinking', descKey: 'chat.slashThinkingDesc' },
  { key: 'compact', nameKey: 'chat.slashCompact', descKey: 'chat.slashCompactDesc' },
  { key: 'session', nameKey: 'chat.slashSession', descKey: 'chat.slashSessionDesc' },
  { key: 'export', nameKey: 'chat.slashExport', descKey: 'chat.slashExportDesc' },
  { key: 'copy', nameKey: 'chat.slashCopy', descKey: 'chat.slashCopyDesc' },
  { key: 'reload', nameKey: 'chat.slashReload', descKey: 'chat.slashReloadDesc' },
  { key: 'login', nameKey: 'chat.slashLogin', descKey: 'chat.slashLoginDesc' },
  { key: 'settings', nameKey: 'chat.slashSettings', descKey: 'chat.slashSettingsDesc' },
]

/** 手打 `/skill:名字 其余文字` 时拆出技能名与剩下的诉求；不是这个形态就返回 null。 */
export function parseSkillCommand(text: string): { name: string; rest: string } | null {
  const m = text.trim().match(/^\/skill:([a-z0-9-]+)(?:\s+([\s\S]*))?$/)
  if (!m) return null
  return { name: m[1], rest: (m[2] || '').trim() }
}
