// 应用状态：引导、设置、主题、运行时、工作目录、SSE 连接。
// SSE 的连接与事件分流放在这里，因为它是全局的（引擎状态、安装进度都在同一通道）。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { setLocale } from '@/i18n'
import { api, streamURL } from '@/api/http'
import { getEndpoint, pickFolder } from '@/api/wails'
import type { AppPrefs, AppRow, Bootstrap, EngineCaps, EngineStatus, RuntimeStatus, SessionRow, WorkspaceRow } from '@/types/api'
import { useChatStore } from './chat'
import { useRefreshStore } from './refresh'
import { useToasts } from '@/composables/useToasts'
import { routeFrame } from './events'
import { readFrame } from '@/utils/sse'
import type { AppProgressLog } from '@/utils/sse'

export type { AppProgressLog }

export const useAppStore = defineStore('app', () => {
  const ready = ref(false)
  const fatal = ref('')
  const version = ref('')
  const contract = ref(0)
  const workspace = ref('')
  const settings = ref<Record<string, string>>({})
  const engines = ref<EngineStatus | null>(null)
  const runtime = ref<RuntimeStatus | null>(null)
  const prefs = ref<AppPrefs | null>(null)
  const workspaces = ref<WorkspaceRow[]>([])
  const sessions = ref<SessionRow[]>([])
  // reDetecting 是「用户点了重新检测」的按钮态；
  // detecting 是「后台装配正在跑」的事实态。两者不是一回事：
  // 前者只持续用户点击的那几秒，后者可能持续整个首屏（解压 115MB）。
  const reDetecting = ref(false)
  // 应用页的数据与安装进度。两者都放全局：概览的横幅、设置页、
  // 应用页三处都看同一份，各拉一份就会出现「这里说装了、那里说没装」。
  const apps = ref<AppRow[]>([])
  const appProgress = ref<AppProgressLog | null>(null)
  // 首次按关闭时的确认框：选「收进托盘」还是「退出」。
  const closePrompt = ref(false)
  const boot = ref<Bootstrap | null>(null)
  let source: EventSource | null = null
  let lastSeq = 0
  let reconnectTimer: number | undefined
  // reconnectDelay 是当前的重连等待。成功连上就复位，
  // 连不上就指数退避——固定间隔重连会在后端未启动时变成请求风暴。
  let reconnectDelay = 1200

  const toasts = useToasts()
  const chat = useChatStore()
  const router = useRouter()

  const theme = computed(() => settings.value['ui.theme'] || 'light')
  /** 内置运行时是否全部就绪（pi 能起来的前提）。 */
  const runtimeReady = computed(() => Boolean(runtime.value?.node_ready && runtime.value?.pi_ready))
  /** 整体是否可用：pi 就绪就能干活。 */
  const hasEngine = computed(() => Boolean(runtime.value?.pi_ready))
  /** 当前能力表；没拿到就按「什么都不能」处理。 */
  const caps = computed<EngineCaps | null>(() => chat.caps ?? engines.value?.engines[0]?.caps ?? boot.value?.engines[0]?.caps ?? null)

  function applyTheme(next: string) {
    document.documentElement.dataset.theme = next === 'dark' ? 'dark' : 'light'
  }

  function applyMotion(reduced: boolean) {
    document.documentElement.dataset.motion = reduced ? 'reduced' : 'full'
  }

  /** 语言跟着设置走：i18n 是全局单例，这里只负责把设置里的值灌进去。 */
  function applyLocale(next: string) {
    setLocale(next === 'en' ? 'en' : 'zh-CN')
  }

  async function load() {
    try {
      await getEndpoint()
      const data = await api.bootstrap()
      boot.value = data
      version.value = data.app_version
      contract.value = data.app_contract
      workspace.value = data.workspace
      settings.value = data.settings ?? {}
      workspaces.value = data.workspaces ?? []
      runtime.value = data.runtime
      prefs.value = data.prefs
      chat.caps = data.engines[0]?.caps ?? null
      applyTheme(theme.value)
      applyMotion(settings.value['ui.reduce_motion'] === '1')
      applyFontScale(Number(settings.value['ui.font_scale'] || 1))
      applyLocale(settings.value['ui.locale'] ?? '')
      ready.value = true
      connect()
      await Promise.all([refreshSessions(), refreshEngines()])
      // 恢复上次会话：只在「用户没指定别的会话」时做，
      // 否则从会话页跳过来会被这里抢回旧会话。
      await restoreLastSession()
    } catch (err) {
      fatal.value = err instanceof Error ? err.message : String(err)
    }
  }

  /**
   * 启动时回到上次那个会话。
   *
   * 只恢复路径，不恢复消息——历史由后端按引擎的会话文件重新投影，
   * 界面不存第二份。会话文件被删掉时后端返回空，这里就安静地落在空会话上。
   */
  async function restoreLastSession() {
    const hash = window.location.hash
    if (hash && hash !== '#/' && !hash.startsWith('#/chat')) return
    if (new URLSearchParams(hash.split('?')[1] ?? '').get('session')) return
    try {
      const current = await api.prefs()
      prefs.value = current
      if (!current.restore_session || !current.last_session_file) return
      await chat.refresh(current.last_session_file)
      const target = `#/chat?session=${current.last_session_file}`
      if (hash !== target) {
        await router.replace({ name: 'chat', query: { session: current.last_session_file } })
      }
    } catch {
      /* 恢复失败不该拦启动：落在空会话上一样能用 */
    }
  }

  function applyFontScale(scale: number) {
    const safe = Number.isFinite(scale) ? Math.min(1.35, Math.max(0.8, scale)) : 1
    document.documentElement.style.setProperty('--wb-fs-scale', String(safe))
  }

  async function refreshSessions() {
    const data = await api.sessions(workspace.value || undefined)
    sessions.value = data.sessions ?? []
  }

  /**
   * 记住当前会话（供下次启动恢复）。
   *
   * 失败不弹提示：这只是「下次回来时的那一下」，不该打断用户正在做的事。
   * 后端在 ChatState 里也会记一次，这里是「切走之后」那条路径的补齐。
   */
  async function rememberSession(path: string): Promise<void> {
    try {
      await api.saveSettings({ 'prefs.last_session_file': path })
    } catch {
      /* 记不住不影响使用 */
    }
  }

  async function refreshEngines() {
    const next = await api.engineStatus()
    engines.value = next
    runtime.value = next.runtime
  }

  // 应用页清单。
  async function loadApps() {
    const data = await api.apps()
    apps.value = data.apps ?? []
    return apps.value
  }

  // 重新探测：设置页的「重新检测」。
  async function detectEngines() {
    reDetecting.value = true
    try {
      await api.detectEngines()
      await refreshEngines()
      await loadApps()
      return engines.value
    } finally {
      reDetecting.value = false
    }
  }

  /**
   * 刷新全部状态：重新探测运行时、重拉会话与模型。
   *
   * 它不做增量：用户点它的场景就是「我怀疑界面上有什么是旧的」，
   * 这时候只刷一半等于没刷。
   */
  async function reloadAll() {
    await detectEngines()
    await Promise.all([refreshSessions(), chat.loadModels(true)])
    if (chat.sessionFile) await chat.refresh(chat.sessionFile)
  }

  async function saveSettings(kv: Record<string, string>) {
    settings.value = { ...settings.value, ...kv }
    if (kv['ui.theme']) applyTheme(kv['ui.theme'])
    if (kv['ui.reduce_motion'] !== undefined) applyMotion(kv['ui.reduce_motion'] === '1')
    if (kv['ui.font_scale'] !== undefined) applyFontScale(Number(kv['ui.font_scale']))
    if (kv['ui.locale'] !== undefined) applyLocale(kv['ui.locale'])
    settings.value = await api.saveSettings(kv)
  }

  async function chooseWorkspace() {
    const picked = await pickFolder(workspace.value)
    if (!picked) return
    await setWorkspace(picked)
  }

  async function setWorkspace(dir: string) {
    const res = await api.setWorkspace(dir)
    workspace.value = res.workspace
    const next = await api.bootstrap()
    workspaces.value = next.workspaces ?? []
    boot.value = next
    await refreshSessions()
    chat.reset()
    await chat.refresh('')
  }

  function connect() {
    if (source) source.close()
    const url = streamURL(lastSeq)
    source = new EventSource(url)
    source.onmessage = (evt) => {
      const frame = readFrame(evt.data)
      if (!frame) return
      if (frame.seq) {
        if (lastSeq && frame.seq > lastSeq + 1) {
          // 断流窗口：seq 跳号说明中间丢了帧，拉一次快照对账
          void reconcile()
        }
        lastSeq = frame.seq
      }
      route(frame.event, frame.data)
    }
    source.onopen = () => {
      reconnectDelay = 1200
    }
    source.onerror = () => {
      source?.close()
      source = null
      window.clearTimeout(reconnectTimer)
      // 退避重连：后端没起来时固定 1.2 秒一次会把日志刷爆，
      // 也让后端刚起来的瞬间还在被上一次的重连风暴打。
      reconnectTimer = window.setTimeout(connect, reconnectDelay)
      reconnectDelay = Math.min(reconnectDelay * 2, 15000)
    }
  }

  async function reconcile() {
    try {
      if (chat.sessionFile || chat.workDir) await chat.refresh(chat.sessionFile)
    } catch {
      /* 引擎没起来时对账会失败，正常 */
    }
  }

  /** 分派一帧事件。分派表在 stores/events.ts。 */
  function route(event: string, data: Record<string, unknown>): void {
    const refresh = useRefreshStore()
    routeFrame(event, data, {
      chat,
      refreshEngines,
      refreshSessions,
      loadApps,
      bumpPlugins: () => refresh.bumpPlugins(),
      bumpSkills: () => refresh.bumpSkills(),
      bumpWorkspace: () => refresh.bumpWorkspace(),
      reconcile,
      appProgress: appProgress.value,
      setAppProgress: (next) => {
        appProgress.value = next
      },
      setClosePrompt: (open) => {
        closePrompt.value = open
      },
      toast: (level, message) => toasts.push(level, message),
    })
  }

  // 跳到设置页（缺运行时的时候这类动作的目标）。
  function openSettings() {
    window.location.hash = '#/settings'
  }

  // 跳到应用页（装 / 升级引擎）。对话页的缺引擎横幅直接指向这里。
  function openApps() {
    window.location.hash = '#/apps'
  }

  function disconnect() {
    source?.close()
    source = null
  }

  return {
    boot,
    ready,
    fatal,
    version,
    contract,
    workspace,
    settings,
    engines,
    runtime,
    prefs,
    reDetecting,
    workspaces,
    sessions,
    closePrompt,
    apps,
    appProgress,
    theme,
    runtimeReady,
    hasEngine,
    caps,
    load,
    refreshSessions,
    rememberSession,
    refreshEngines,
    detectEngines,
    reloadAll,
    loadApps,
    saveSettings,
    chooseWorkspace,
    setWorkspace,
    openSettings,
    openApps,
    disconnect,
  }
})
