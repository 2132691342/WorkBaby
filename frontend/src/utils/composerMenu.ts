// 输入区斜杠菜单的命令表与类型：菜单组件与命令执行两侧共用，放在组件外避免
// <script setup> 的「不可导出」限制，也避免两边各抄一份命令清单。

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

/** 一条待发送附件。 */
export interface PendingFile {
  /** 相对工作区路径（粘贴的图片为空） */
  path: string
  /** 显示名 */
  name: string
  /** 是否落在工作区之外（提交前必须先解决） */
  outside: boolean
  /** 粘贴进来的图片 base64 */
  image?: string
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
