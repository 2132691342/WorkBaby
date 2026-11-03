// 输入区的内置斜杠命令：界面自己就能做完的动作，与引擎命令分开——
// 前者由界面执行，后者只是往输入框填文字，生效靠引擎那一轮。
// 只列「点了真的会发生什么」的条目；只活在终端 TUI 上的命令不列。
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/stores/chat'
import { useAppStore } from '@/stores/app'
import { api } from '@/api/http'
import type { EngineCaps } from '@/types/api'

/** 内置命令的键。新增一条要同时加 i18n 文案与 run 分支。 */
export type BuiltinKey =
  | 'new'
  | 'model'
  | 'thinking'
  | 'compact'
  | 'copy'
  | 'export'
  | 'exporthtml'
  | 'reload'
  | 'clone'

interface BuiltinDef {
  key: BuiltinKey
  nameKey: string
  descKey: string
  /** 有能力位要求的命令：引擎不支持就不进菜单，绝不渲染灰项。 */
  cap?: keyof EngineCaps
}

const DEFS: BuiltinDef[] = [
  { key: 'new', nameKey: 'chat.slashNew', descKey: 'chat.slashNewDesc' },
  { key: 'model', nameKey: 'chat.slashModel', descKey: 'chat.slashModelDesc' },
  { key: 'thinking', nameKey: 'chat.slashThinking', descKey: 'chat.slashThinkingDesc' },
  { key: 'compact', nameKey: 'chat.slashCompact', descKey: 'chat.slashCompactDesc' },
  { key: 'copy', nameKey: 'chat.slashCopy', descKey: 'chat.slashCopyDesc' },
  { key: 'export', nameKey: 'chat.slashExport', descKey: 'chat.slashExportDesc' },
  { key: 'exporthtml', nameKey: 'chat.slashExportHTML', descKey: 'chat.slashExportHTMLDesc', cap: 'export_html' },
  { key: 'reload', nameKey: 'chat.slashReload', descKey: 'chat.slashReloadDesc' },
  { key: 'clone', nameKey: 'chat.slashClone', descKey: 'chat.slashCloneDesc', cap: 'session_clone' },
]

/**
 * 内置命令的清单与执行。
 *
 * 执行返回的错误话术由调用方显示——这里不弹提示，
 * 否则「谁在提示」会分散到三个文件里。
 */
export function useBuiltinCommands() {
  const chat = useChatStore()
  const app = useAppStore()
  const { t } = useI18n()

  /** 菜单里要显示的内置命令（已翻译，按引擎能力位过滤）。 */
  function items(): { key: BuiltinKey; name: string; desc: string }[] {
    const caps = chat.caps
    return DEFS.filter((d) => !d.cap || caps?.[d.cap]).map((d) => ({
      key: d.key,
      name: t(d.nameKey),
      desc: t(d.descKey),
    }))
  }

  /** 执行一条内置命令。返回成功提示话术（空串表示不需要提示）。 */
  async function run(key: BuiltinKey): Promise<string> {
    switch (key) {
      case 'new':
        await chat.newChat()
        return t('chat.newChatStarted')

      case 'clone':
        await chat.cloneChat()
        return t('chat.slashCloneDone')

      case 'compact':
        if (!chat.sessionFile) return t('chat.slashNeedSession')
        await chat.compact()
        return t('chat.slashCompactDone')

      case 'model':
        // 模型面板就在输入框底栏上：直接叫它打开，比再弹一层少一步。
        chat.pickerRequest = 'model'
        return ''

      case 'thinking':
        chat.pickerRequest = 'thinking'
        return ''

      case 'copy':
        return copyLastAnswer()

      case 'export':
        return exportMarkdown()

      case 'exporthtml':
        return exportHTML()

      case 'reload':
        await app.reloadAll()
        return t('chat.slashReloadDone')
    }
  }

  /** 复制最后一条助手回答的纯文本。 */
  async function copyLastAnswer(): Promise<string> {
    const turns = chat.history
    for (let i = turns.length - 1; i >= 0; i--) {
      const turn = turns[i]
      if (!turn || turn.role !== 'assistant') continue
      const text = turn.blocks
        .filter((b): b is { kind: 'text'; text: string } => b.kind === 'text' && Boolean(b.text))
        .map((b) => b.text)
        .join('\n')
        .trim()
      if (!text) break
      await navigator.clipboard.writeText(text)
      return t('chat.slashCopyDone')
    }
    return t('chat.slashCopyEmpty')
  }

  /**
   * 导出当前会话为 Markdown。
   *
   * 用浏览器下载而不是系统保存对话框：内容是我们自己拼的，走 Blob 少一次
   * 跨层调用；用户还能从下载目录里找到它。
   */
  async function exportMarkdown(): Promise<string> {
    if (!chat.sessionFile) return t('chat.slashNeedSession')
    const lines: string[] = []
    for (const turn of chat.history) {
      lines.push(turn.role === 'user' ? '## 我' : '## 助手')
      lines.push('')
      for (const block of turn.blocks) {
        if (block.kind === 'text' && block.text) lines.push(block.text)
      }
      lines.push('')
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/markdown;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `session-${Date.now()}.md`
    link.click()
    URL.revokeObjectURL(url)
    return t('chat.slashExportDone')
  }

  /**
   * 引擎导出的单文件 HTML（pi 的 export_html RPC）：样式、代码高亮全部内联，
   * 比前端拼的 Markdown 更接近终端里的原貌。文件名由后端按会话名给。
   */
  async function exportHTML(): Promise<string> {
    if (!chat.sessionFile) return t('chat.slashNeedSession')
    const res = await api.exportSessionHTML(chat.sessionFile)
    const blob = new Blob([res.content], { type: 'text/html;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = res.file_name
    link.click()
    URL.revokeObjectURL(url)
    return t('chat.slashExportHTMLDone')
  }

  return { items, run }
}
