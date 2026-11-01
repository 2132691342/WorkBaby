// 输入区的内置命令动作：斜杠菜单里每条 `/xxx` 点下去真正做的事。
// 抽出来的理由只有一条——这些动作与输入框的键盘 / 菜单状态无关，
// 留在组件里会把 ChatInput 顶到 500 行以上。
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import * as api from '../api'
import { pickFileSave } from '../api/shell'
import { i18n } from '../i18n'
import { useChatStore } from '../stores/chat'
import { useSessionStore } from '../stores/session'
import { useSettingsStore } from '../stores/settings'
import { useToastStore } from '../stores/toast'
import type { BuiltinKey } from '../utils/composerMenu'

/** 命令用到的界面出口：都由调用方（ChatInput）注入，composable 不碰 DOM。 */
export interface CommandHooks {
  /** 开新对话 */
  newSession: () => void
  /** 打开模型选择条 */
  showModel: () => void
  /** 打开思考档条 */
  showThinking: () => void
}

export function useComposerCommands(hooks: CommandHooks) {
  const t = i18n.global.t
  const router = useRouter()
  const chat = useChatStore()
  const session = useSessionStore()
  const settings = useSettingsStore()
  const toast = useToastStore()

  /** 正在执行的内置命令：执行中不接受第二条，避免两次压缩同时开跑。 */
  const busy = ref('')

  /** 取当前会话 id；没有就提示并返回空串，调用方按空串提前退出。 */
  function needSession(): string {
    const sid = session.currentId
    if (!sid) toast.info(t('chat.slashNeedSession'))
    return sid || ''
  }

  async function compact() {
    const sid = needSession()
    if (!sid) return
    if (chat.running) {
      toast.bad(t('chat.slashBusy'))
      return
    }
    await api.chat.compact(sid)
    toast.ok(t('chat.slashCompactDone'))
    await session.refresh()
  }

  function showSessionInfo() {
    const s = session.current
    if (!s) {
      toast.info(t('chat.slashNeedSession'))
      return
    }
    toast.info(
      t('chat.slashSessionInfo', {
        n: s.session_message_count,
        tokens: s.session_total_tokens,
        model: s.session_model || '—',
      }),
    )
  }

  async function exportHTML() {
    const sid = needSession()
    if (!sid) return
    const safe = (session.current?.session_title || 'session').replace(/[\\/:*?"<>|]/g, '_')
    const target = await pickFileSave(t('chat.slashExportTitle'), `${safe}.html`, '*.html')
    if (!target) return
    const saved = await api.sessions.exportHTML(sid, target)
    toast.ok(t('chat.slashExportDone', { path: saved }))
  }

  async function copyLastAnswer() {
    const last = [...session.messages]
      .reverse()
      .find((m) => m.message_role === 'assistant' && m.message_content?.trim())
    if (!last?.message_content) {
      toast.info(t('chat.slashCopyEmpty'))
      return
    }
    try {
      await navigator.clipboard.writeText(last.message_content)
      toast.ok(t('chat.slashCopyDone'))
    } catch {
      toast.bad(t('common.copyFailed'))
    }
  }

  /** 跑一条内置命令；技能类候选不走这里（由调用方拼 `/skill:名` 前缀）。 */
  async function run(key: BuiltinKey) {
    if (busy.value) return
    switch (key) {
      case 'new':
        hooks.newSession()
        return
      case 'model':
        hooks.showModel()
        return
      case 'thinking':
        hooks.showThinking()
        return
      case 'session':
        showSessionInfo()
        return
      case 'login':
        void router.push({ name: 'settings', params: { tab: 'providers' } })
        return
      case 'settings':
        void router.push({ name: 'settings' })
        return
      case 'compact':
      case 'export':
      case 'copy':
      case 'reload':
        break
    }
    busy.value = key
    try {
      if (key === 'compact') await compact()
      else if (key === 'export') await exportHTML()
      else if (key === 'copy') await copyLastAnswer()
      else if (key === 'reload') {
        await settings.loadSkills()
        toast.ok(t('chat.slashReloadDone'))
      }
    } catch (e) {
      toast.bad(t('chat.slashFailed', { msg: (e as Error)?.message || '' }))
    } finally {
      busy.value = ''
    }
  }

  return { busy, run }
}
