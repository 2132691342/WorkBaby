// 开发态预览：把 window.runtime / axios 换成假实现，`npm run dev` 就能在
// 普通浏览器里看到成品界面，改视觉不必每次 wails build 开窗。
// 生产构建下 import.meta.env.DEV 为 false，整段被摇掉，不进包。
import http from '../api/http'
import * as fx from './fixtures'

type Handler = (body: unknown, query: URLSearchParams, method: string) => unknown

const now = Date.now()

function json(data: unknown) {
  return { code: 0, message: '', data }
}

// 会话 / 技能 / 文档在内存里增删：预览时能真的点新建、点删除。
const state = {
  sessions: [...fx.sessions],
  skills: [...fx.skills],
  docs: [...fx.docs],
  providers: [...fx.providers],
  tools: [...fx.tools],
  settings: { ...fx.boot.settings },
  empty: new URLSearchParams(location.search).get('empty') === '1',
}

function newID(prefix: string) {
  return `${prefix}_${Date.now().toString(36).toUpperCase()}`
}

const routes: Array<[string, Handler]> = [
  ['/bootstrap', () => fx.boot],
  // 空态预览（?empty=1）：列表为空且拒绝创建，界面才会停在欢迎页；
  // 非 POST 的 /sessions 也可能带尾部动作（delete/rename 等），那些走后面的兜底。
  [
    '/sessions',
    (_body, _q, method) => {
      if (method === 'POST') {
        // 以夹具会话为模板补齐必填字段，预览里点「新对话」能得到完整的一条。
        // 空态预览不把它放进列表（界面停在欢迎页），但返回体必须是合法会话，
        // 调用方紧接着读它的 id，返回 null 会让整个界面崩掉。
        const s = { ...fx.sessions[0], id: newID('SESSION'), title: '新对话' }
        if (!state.empty) state.sessions = [s, ...state.sessions]
        return s
      }
      return state.empty ? [] : state.sessions
    },
  ],
  ['/chat/send', () => ({ run_id: 'RUN_1', entry_id: 'ENTRY_1', session_id: state.sessions[0]?.id || '' })],
  ['/chat/stop', () => true],
  ['/chat/steer', () => true],
  ['/stats', () => (state.empty ? fx.emptyStats : fx.stats(14))],
  ['/providers', () => state.providers],
  ['/providers/models', () => ['claude-sonnet-4-5', 'claude-haiku-4-5']],
  // 运行时状态：内置 Python 就绪 + PowerShell 用系统兜底，覆盖两种展示
  ['/runtime', () => fx.runtimeInfo],
  ['/runtime/redetect', () => fx.runtimeInfo],
  // 模型能力：带 provider_id 才读得到用户配置（演示模型级 1024000 窗口的场景）
  [
    '/models/capability',
    (_b, q) => ({
      id: q.get('model') || '',
      context_window: 1_024_000,
      max_output: 32_768,
      thinking: true,
      vision: false,
      tool_call: true,
      known: Boolean(q.get('provider_id')),
      note: q.get('provider_id') ? '' : '本地没有这个模型的资料，窗口按估算显示',
    }),
  ],
  ['/tools', () => state.tools],
  ['/skills', () => state.skills],
  ['/knowledge/docs', () => state.docs],
  ['/knowledge/reindex', () => ({ reindexed: state.docs.length })],
  [
    '/knowledge/search',
    () => ({
      hits: [{ doc_id: 'D1', title: '产品手册-v3.md', seq: 1, content: '产品支持按席位购买，最少 5 席。', score: 0.82 }],
    }),
  ],
]

function route(path: string, body: unknown, query: URLSearchParams, method = 'GET'): unknown {
  const hit = routes.find(([pattern]) => pattern === path)
  if (hit) return hit[1](body, query, method)

  // 会话详情：返回 fixtures 里的消息历史（含工具调用与思考块），预览时能看到完整界面
  if (/^\/sessions\/[^/]+$/.test(path)) {
    const id = path.split('/')[2]
    const session = state.sessions.find((s) => s.id === id)
    return { session, messages: fx.messages[id] || [] }
  }

  // 带 :id 的动作一律后缀匹配，路由表就不用为每个动词各写一条。
  const parts = path.split('/')
  const tail = parts[parts.length - 1] || ''
  const id = parts[parts.length - 2] || ''
  if (tail === 'delete') {
    if (path.startsWith('/sessions')) state.sessions = state.sessions.filter((s) => s.id !== id)
    if (path.startsWith('/skills')) state.skills = state.skills.filter((s) => s.id !== id)
    if (path.startsWith('/knowledge')) state.docs = state.docs.filter((d) => d.id !== id)
    return true
  }
  if (tail === 'rename') return true
  if (tail === 'toggle') {
    const on = Boolean((body as { enabled?: boolean })?.enabled)
    if (path.startsWith('/tools')) {
      const t = state.tools.find((x) => x.name === id)
      if (t) t.enabled = on
    } else {
      const s = state.skills.find((x) => x.id === id)
      if (s) s.enabled = on
    }
    return true
  }
  if (tail === 'content') {
    const s = state.skills.find((x) => x.id === id)
    return { content: `## 什么时候用\n\n${s?.description || ''}\n\n## 怎么做\n\n1. 读取相关资料\n2. 按模板整理\n3. 交给用户确认` }
  }
  if (tail === 'default') return true
  if (tail === 'test') return { ok: true, model: 'claude-sonnet-4-5', detail: '连接正常' }
  if (tail === 'reveal') return { api_key: 'sk-preview-1234567890abcdef' }
  if (tail === 'update') return state.providers[0]
  if (tail === 'branch') return true
  if (tail === 'model' || tail === 'permission') return true

  if (path === '/skills' && body) {
    const b = body as { name: string; description: string }
    const skill = {
      id: `${b.name}@global`,
      name: b.name,
      description: b.description,
      location: `skills/${b.name}`,
      source: 'global',
      enabled: true,
    }
    state.skills = [...state.skills, skill]
    return skill
  }
  if (path === '/skills/import') {
    state.skills = [
      ...state.skills,
      {
        id: 'imported-demo@global',
        name: 'imported-demo',
        description: '从文件夹导入的技能',
        location: 'skills/imported-demo',
        source: 'global',
        enabled: true,
      },
    ]
    return { imported: 1, skipped: [] }
  }
  if (path === '/knowledge/docs/add') {
    const paths = ((body as { paths?: string[] })?.paths || []).map((p, i) => ({
      id: newID('DOC'),
      path: p.split(/[\\/]/).pop() || p,
      title: p.split(/[\\/]/).pop() || p,
      ext: 'md',
      size: 12_000,
      chunks: 6,
      status: 'indexed',
      created_at: now + i,
    }))
    state.docs = [...state.docs, ...paths]
    return { added: paths.length }
  }
  if (path === '/settings') {
    const b = body as { key: string; value: string }
    if (b?.key) state.settings[b.key] = b.value
    return true
  }
  return null
}

/** Wails 宿主注入的两个全局，只在预览时我们自己造一份。 */
type Listener = (data: unknown) => void
interface PreviewRuntime {
  EventsOnMultiple: (name: string, cb: Listener) => () => void
  EventsOn: (name: string, cb: Listener) => () => void
  EventsOnce: (name: string, cb: Listener) => () => void
  EventsOff: (name: string) => boolean
  EventsEmit: (name: string, data: unknown) => void
  LogPrint: (msg: string) => void
  WindowMinimise: () => void
  WindowToggleMaximise: () => void
  WindowHide: () => void
  Quit: () => void
}

export function installPreview() {
  // 事件总线要真的能派发：bootstrap 靠 app:ready 握手才肯放行界面
  const listeners = new Map<string, Listener[]>()
  const on = (name: string, cb: Listener) => {
    listeners.set(name, [...(listeners.get(name) || []), cb])
    return () => listeners.set(name, (listeners.get(name) || []).filter((f) => f !== cb))
  }

  const runtime: PreviewRuntime = {
    EventsOnMultiple: on,
    EventsOn: on,
    EventsOnce: on,
    EventsOff: (name) => listeners.delete(name),
    EventsEmit: (name, data) => (listeners.get(name) || []).forEach((f) => f(data)),
    LogPrint: () => undefined,
    WindowMinimise: () => undefined,
    WindowToggleMaximise: () => undefined,
    WindowHide: () => undefined,
    // 预览里没有真实窗口生命周期：关闭按钮触发的是外壳的 Quit 判定，这里只吞掉调用
    Quit: () => undefined,
  }

  const w = window as unknown as { runtime: PreviewRuntime; go: unknown }
  w.runtime = runtime
  w.go = {
    main: {
      App: {
        OpenFileDialog: async () => 'C:\\Users\\demo\\工作\\季度总结.md',
        OpenDirectoryDialog: async () => 'C:\\Users\\demo\\工作\\skills',
        ForceQuit: () => undefined,
      },
    },
  }

  http.defaults.adapter = async (cfg) => {
    const url = new URL(String(cfg.url || '/'), 'http://preview.local')
    let body: unknown = cfg.data
    if (typeof body === 'string') {
      try {
        body = JSON.parse(body)
      } catch {
        /* 非 JSON 请求体原样透传 */
      }
    }
    return {
      data: json(route(url.pathname, body, url.searchParams, (cfg.method || 'GET').toUpperCase())),
      status: 200,
      statusText: 'OK',
      headers: {},
      config: cfg,
    }
  }

  // 与 Go 侧 repeatReady 同策略：模块图是异步加载的，单次 0ms 广播可能早于
  // bootstrap 注册监听，之后就永远等不到端口（表现：整页空白）
  for (let i = 0; i < 20; i++) {
    setTimeout(() => runtime.EventsEmit('app:ready', { server_port: 5173 }), i * 100)
  }
}

if (import.meta.env.DEV) installPreview()
