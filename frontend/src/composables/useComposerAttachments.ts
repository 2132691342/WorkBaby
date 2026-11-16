// 输入区的附件来源：粘贴图片 / 回形针选文件 / @ 引用工作区文件 / @ 引用历史会话。
// 四种来源共用一张待发送列表，列表本身在 useAttachments 里；
// 这里管的是「怎么把东西弄进列表」以及弄完之后输入框该怎么收尾。
// 拆出来是因为这四件事与发送、键盘、菜单无关，混在输入区里会让它变成一袋杂活。
import { nextTick, ref } from 'vue'
import type { Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAttachments, baseName, toWorkspaceRelative } from '@/composables/useAttachments'
import { pickFiles } from '@/api/wails'
import { api } from '@/api/http'

export function useComposerAttachments(opts: {
  /** 当前草稿：@ 引用落成附件后要把触发菜单的那段 @xxx 抹掉 */
  draft: Ref<string>
  /** 关掉弹出菜单：引用落成附件后菜单就该收起 */
  closeMenu: () => void
  /** 输入框重新量高 */
  autosize: () => void
  /** 焦点放回输入框 */
  focus: () => void
}) {
  const app = useAppStore()
  const { t } = useI18n()

  // 附件：粘贴图片 / 回形针选文件 / @ 引用工作区文件，三种来源一个列表。
  const attachments = useAttachments()
  // 附件加不进来的原因：逐条汇总给用户，不打断别的操作。
  const attachError = ref('')

  /** 引用落成附件后的收尾：抹掉触发菜单的那段 @xxx，收起菜单，重算高度并回焦点。 */
  function settleReference(): void {
    opts.draft.value = opts.draft.value.replace(/@\S*$/, '').trimEnd()
    opts.closeMenu()
    void nextTick(() => {
      opts.autosize()
      opts.focus()
    })
  }

  /** @ 菜单选中了一个工作区文件：落成附件条目。 */
  function onPickFile(relPath: string, name: string): void {
    const err = attachments.addWorkspaceFile(relPath, name)
    if (err) {
      attachError.value = err
      return
    }
    settleReference()
  }

  // 引用一段历史会话当上下文：拉它的逐条正文，折成一块给模型读。
  // 会话可能很长，只留最近一段（约 4000 字）——引用是「接着上次说」，不是把整份历史重放一遍。
  async function onPickSession(sessionPath: string, title: string): Promise<void> {
    try {
      const detail = await api.sessionDetail(sessionPath)
      const role = (kind: string) => (kind === 'user' ? '用户' : kind === 'assistant' ? '助手' : '工具')
      const lines = (detail.items ?? [])
        .filter((it) => it.item_text && it.item_text.trim())
        .map((it) => `${role(it.item_kind)}：${it.item_text?.trim()}`)
      const body = `[引用历史会话「${title}」]\n${lines.join('\n').slice(-4000)}`
      const err = attachments.addSession(title, body)
      if (err) {
        attachError.value = err
        return
      }
    } catch (err) {
      attachError.value = err instanceof Error ? err.message : String(err)
      return
    }
    settleReference()
  }

  /** 粘贴：把图片收进附件列表，文本照常进输入框。 */
  async function onPaste(event: ClipboardEvent): Promise<void> {
    const errors = await attachments.addFromClipboard(event.clipboardData)
    if (errors.length) {
      attachError.value = errors.join('；')
      return
    }
    // 有图片时吃掉这次粘贴，避免同一张图既进附件又变成一段二进制文本；
    // 没有图片就放行——普通文本粘进输入框是正常操作。
    if (attachments.images.value.length) event.preventDefault()
  }

  /** 回形针：选文件。工作区内的变成相对路径引用，工作区外的给绝对路径。 */
  async function browseFiles(): Promise<void> {
    const picked = await pickFiles(t('chat.attachPick'))
    if (!picked.length) return
    const errors: string[] = []
    for (const path of picked) {
      const rel = toWorkspaceRelative(path, app.workspace)
      const err = attachments.addWorkspaceFile(rel ?? path, baseName(path))
      if (err) errors.push(err)
    }
    if (errors.length) attachError.value = errors.join('；')
    void nextTick(() => opts.focus())
  }

  return { attachments, attachError, onPickFile, onPickSession, onPaste, browseFiles }
}
