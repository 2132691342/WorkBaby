// 应用状态：引导、设置、主题、引擎、工作目录、SSE 连接。
// SSE 的连接与事件分流放在这里，因为它是全局的（引擎状态、安装进度都在同一通道）。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { api, streamURL } from '@/api/http'
import { getEndpoint, pickFolder } from '@/api/shell'
import type { AppRow, Bootstrap, EngineCaps, EngineKind, EngineStatus, SessionRow, WorkspaceRow } from '@/types/api'
import { useChatStore } from './chat'
import { useAgentsStore } from './agents'
import { useToasts } from '@/composables/useToasts'
import { routeFrame } from './events'
import { readFrame } from '@/utils/sse'
import type { AppProgressLog } from '@/utils/sse'

export type { AppProgressLog }

export const useAppStore = defineStore('app', () => {
  const ready = ref(false)
  const fatal = ref('')
  const version = ref('')
  const workspace = ref('')
  const profile = ref('')
  const settings = ref<Record<string, string>>({})
  const engines = ref<EngineStatus | null>(null)
  const active = ref<EngineKind>('pi')
  const workspaces = ref<WorkspaceRow[]>([])
  const sessions = ref<SessionRow[]>([])
  // reDetecting 是「用户点了重新检测」的按钮态；
  // detecting 是「后台探测正在跑」的事实态。两者不是一回事：
  // 前者只持续用户点击的那几秒，后者可能持续整个首屏。
  const reDetecting = ref(false)
  // 应用页的数据与安装进度。两者都放全局：缺引擎横幅、设置页引擎卡、
  // 应用页三处都看同一份，各拉一份就会出现「这里说装了、那里说没装」。
  const apps = ref<AppRow[]>([])
  const appProgress = ref<Record<string, AppProgressLog>>({})

  // 引擎请求浏览器完成的一次授权。它不弹窗、不自动打开，只在对话页顶部
  // 挂一条可复制的地址——默认浏览器可能打不开，用户需要能看到原文。
  const pendingLink = ref<{ url: string; label: string; instructions: string } | null>(null)

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
  const agents = useAgentsStore()
  const router = useRouter()

  const theme = computed(() => settings.value['ui.theme'] || 'light')
  /** 探测到的引擎清单（按固定顺序 omp → pi）。 */
  const installed = computed(() => engines.value?.engines.filter((e) => e.installed) ?? [])
  /** 全都没有引擎时，输入区要禁用并给安装命令。 */
  const hasEngine = computed(() => installed.value.length > 0)
  /** 某引擎的能力表；没探测到就按「什么都不能」处理。 */
  function capsOf(kind: EngineKind): EngineCaps | null {
    const found = engines.value?.engines.find((e) => e.kind === kind)
    return found?.caps ?? boot.value?.caps?.[kind] ?? null
  }
  /** 安装命令原文（缺引擎时给用户复制）。 */
  function installHint(kind: EngineKind): string {
    return engines.value?.install?.[kind] ?? boot.value?.install?.[kind] ?? ''
  }

  function applyTheme(next: string) {
    document.documentElement.dataset.theme = next === 'dark' ? 'dark' : 'light'
  }

  function applyMotion(reduced: boolean) {
    document.documentElement.dataset.motion = reduced ? 'reduced' : 'full'
  }

  async function load() {
    try {
      await getEndpoint()
      const data = await api.bootstrap()
      boot.value = data
      version.value = data.version
      workspace.value = data.workspace
      profile.value = data.profile
      settings.value = data.settings ?? {}
      workspaces.value = data.workspaces ?? []
      active.value = data.active ?? 'pi'
      setActiveKind(active.value)
      chat.kind = data.active ?? 'pi'
      chat.caps = data.caps?.[data.active] ?? null
      applyTheme(theme.value)
      applyMotion(settings.value['ui.reduce_motion'] === '1')
      applyFontScale(Number(settings.value['ui.font_scale'] || 1))
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
      const prefs = await api.prefs()
      if (!prefs.restore_session || !prefs.last_session_file) return
      if (prefs.last_engine_kind) {
        active.value = prefs.last_engine_kind
        setActiveKind(prefs.last_engine_kind)
        chat.kind = prefs.last_engine_kind
        chat.caps = capsOf(prefs.last_engine_kind)
      }
      await chat.refresh(prefs.last_session_file)
      const current = `#/chat?session=${prefs.last_session_file}`
      if (hash !== current) {
        await router.replace({ name: 'chat', query: { session: prefs.last_session_file } })
      }
    } catch {
      /* 恢复失败不该拦启动：落在空会话上一样能用 */
    }
  }

  function applyFontScale(scale: number) {
    const safe = Number.isFinite(scale) ? Math.min(1.35, Math.max(0.8, scale)) : 1
    document.documentElement.style.setProperty('--wb-fs-scale', String(safe))
  }

  // 会话列表跟当前引擎走：两个引擎的会话文件不能互换接着聊，
  // 混一张表会让人以为点开就能继续。按引擎各缓存一份，
  // 切引擎时先摆上目标引擎的列表（不等往返），后台再补新的。
  const sessionCache = ref<Record<EngineKind, SessionRow[]>>({ omp: [], pi: [] })

  async function refreshSessions() {
    const target = chat.kind
    const data = await api.sessions(workspace.value || undefined, target)
    const rows = data.sessions ?? []
    sessionCache.value = { ...sessionCache.value, [target]: rows }
    // 请求飞的半秒里用户又切了：不能拿旧引擎的列表覆盖当前引擎的。
    if (chat.kind === target) sessions.value = rows
  }

  /** 切引擎时立刻把列表换过去：会话列表是一个一直看得见的栏目。 */
  function showCachedSessions(kind: EngineKind) {
    sessions.value = sessionCache.value[kind] ?? []
  }

  /**
   * 记住当前会话（供下次启动恢复）。
   *
   * 失败不弹提示：这只是「下次回来时的那一下」，不该打断用户正在做的事。
   * 后端在 ChatState 里也会记一次，这里是“切走之后”那条路径的补齐。
   */
  async function rememberSession(path: string) {
    try {
      await api.rememberSession(chat.kind, path)
    } catch {
      /* 记不住不影响使用 */
    }
  }

  async function refreshEngines() {
    const next = await api.engineStatus()
    engines.value = next
    // 记住这一份结论的探测时刻：它是「要不要再拉一次」的判据。
    // 事件帧可能丢（探测早于 SSE 连接时），比对时间戳是唯一不依赖帧的兜底。
    lastDetectedAt.value = next.detected_at ?? 0
  }

  /**
   * 后端那边探测完成到哪一刻了。
   *
   * 它与 detecting 配合做兜底：前者说「后台在跑」，后者说「跑到哪一轮」。
   * 概览页据此决定「拿到的这份是不是已经过期了」——engine:detected 那一帧
   * 丢了也能自己回来。
   */
  const lastDetectedAt = ref(0)

  /**
   * 引擎探测进行中（含「从没探过」）。
   *
   * 它回答「界面能不能说『没装』」：还在探的时候说没装是撒谎，
   * 而首发时探测要几秒（pi 的 --help 起一次 Node），那段窗口正好是用户第一眼看到的。
   */
  const detecting = computed(() => engines.value?.detecting ?? true)

  // 应用页清单。refresh 为真时忽略后端缓存（重查上游最新版）。
  async function loadApps(refresh = false) {
    const data = await api.apps(refresh)
    apps.value = data.apps ?? []
    return apps.value
  }

  // 重新探测：设置页的「重新检测」。pi 的探测要起一次 Node，所以要给 loading。
  async function detectEngines() {
    reDetecting.value = true
    try {
      engines.value = await api.detectEngines()
      return engines.value
    } finally {
      reDetecting.value = false
    }
  }

  /**
   * 刷新全部状态：重新探测引擎、重拉会话与用量。
   *
   * 这是 /reload 那条内置命令的实现。它不做增量：用户点它的场景就是
   * 「我怀疑界面上有什么是旧的」，这时候只刷一半等于没刷。
   */
  async function reloadAll() {
    await detectEngines()
    await Promise.all([refreshSessions(), chat.loadModels()])
    if (chat.sessionFile) await chat.refresh(chat.sessionFile)
  }

  // 切当前引擎：写设置 + 换对话通道。切引擎不切目录、不切会话。
  // 先切本地再落盘：后端只在下一句话发出去时才读它，点击不该等写设置的往返。
  async function setEngine(kind: EngineKind) {
    if (kind === active.value) return
    active.value = kind
    // 先同步过滤目标再切通道：反过来的话，切换期间到达的旧引擎帧会被放进来。
    setActiveKind(kind)
    showCachedSessions(kind)
    void chat.switchEngine(kind)
    // 会话列表跟着引擎走：它是一份非当前引擎的数据，切完就该换。
    void refreshSessions()
    // 落盘失败不影响这一次切换：下次启动还能用上一次的选择。
    void api.setEngine(kind).catch(() => undefined)
  }

  async function saveSettings(kv: Record<string, string>) {
    settings.value = { ...settings.value, ...kv }
    if (kv['ui.theme']) applyTheme(kv['ui.theme'])
    if (kv['ui.reduce_motion'] !== undefined) applyMotion(kv['ui.reduce_motion'] === '1')
    if (kv['ui.font_scale'] !== undefined) applyFontScale(Number(kv['ui.font_scale']))
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
    routeFrame(event, data, currentActiveKind, {
      chat,
      agents,
      refreshEngines,
      refreshSessions,
      loadApps,
      reconcile,
      appProgress: appProgress.value,
      setAppProgress: (kind, next) => {
        appProgress.value = { ...appProgress.value, [kind]: next }
      },
      setClosePrompt: (open) => {
        closePrompt.value = open
      },
      setPendingLink: (link) => {
        pendingLink.value = link
      },
      toast: (level, message) => toasts.push(level, message),
      onStatusFrame: (kind, data) => dispatchStatus(kind, data),
      onProgressFrame: (data) => dispatchProgress(data),
    })
  }

  /** 应用安装进度帧的订阅表（插件页画逐行 npm 输出用）。 */
  const progressHandlers = new Set<(data: Record<string, unknown>) => void>()

  /** 订阅安装进度帧；返回退订函数。 */
  function onProgress(handler: (data: Record<string, unknown>) => void): () => void {
    progressHandlers.add(handler)
    return () => progressHandlers.delete(handler)
  }

  /** 广播一帧安装进度给已登记的面板。 */
  function dispatchProgress(data: Record<string, unknown>): void {
    for (const handler of [...progressHandlers]) handler(data)
  }

  /**
   * 资源页订阅表：插件页与技能页在挂载时登记刷新函数。
   * 寄存器放在 store 里，因为 SSE 只有一条——页面各自新建 EventSource 会拿到重复帧。
   */
  const statusHandlers = new Set<(kind: 'plugin' | 'skill', data: Record<string, unknown>) => void>()

  /** 订阅资源状态帧；返回退订函数。 */
  function onStatus(handler: (kind: 'plugin' | 'skill', data: Record<string, unknown>) => void): () => void {
    statusHandlers.add(handler)
    return () => statusHandlers.delete(handler)
  }

  function dispatchStatus(kind: 'plugin' | 'skill', data: Record<string, unknown>): void {
    for (const handler of [...statusHandlers]) handler(kind, data)
  }

  // 跳到设置页（缺引擎时「去设置」这类动作的目标）。
  function openSettings() {    window.location.hash = '#/settings'
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
    workspace,
    profile,
    settings,
    engines,
    active,
    detecting,
    lastDetectedAt,
    reDetecting,
    workspaces,
    sessions,
    pendingLink,
    closePrompt,
    apps,
    appProgress,
    theme,
    installed,
    hasEngine,
    capsOf,
    installHint,
    load,
    refreshSessions,
    rememberSession,
    refreshEngines,
    detectEngines,
    reloadAll,
    loadApps,
    setEngine,
    saveSettings,
    chooseWorkspace,
    setWorkspace,
    openSettings,
    openApps,
    disconnect,
    onStatus,
    onProgress,
  }
})

// ---------------- SSE 载荷的类型边界 ----------------
// 窄化逻辑在 utils/sse.ts，分派表在 stores/events.ts。
// 这里只回答「这一帧属于谁」。

/** 当前正在看的引擎。由 store 在切引擎时同步。 */
let currentActiveKind = ''

/** 同步当前引擎，供帧过滤读。 */
export function setActiveKind(kind: string) {
  currentActiveKind = kind
}
