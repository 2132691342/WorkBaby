// 开发态预览：让 `npm run dev` 在普通浏览器里就能看到真实界面。
//
// 桌面端跑的是 Wails 宿主，界面却完全由 HTTP + 事件驱动，所以只要把
// window.runtime / window.go / axios 适配器换成假实现，浏览器里看到的就是成品。
// 目的只有一个：改视觉时能当场看到效果，不用每次都 wails build 再开窗。
//
// 生产构建下 import.meta.env.DEV 为 false，整个函数体被摇掉，不进包。
import http from '../api/http'
import * as fx from './fixtures'

type Handler = (body: unknown, query: URLSearchParams) => unknown

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
  settings: { ...fx.boot.settings },
  empty: new URLSearchParams(location.search).get('empty') === '1',
}

function newID(prefix: string) {
  return `${prefix}_${Date.now().toString(36).toUpperCase()}`
}

const routes: Array<[string, Handler]> = [
  ['/bootstrap', () => fx.boot],
  ['/sessions', () => state.sessions],
  ['/chat/send', () => ({ run_id: 'RUN_1', entry_id: 'ENTRY_1', session_id: state.sessions[0].id })],
  ['/chat/stop', () => true],
  ['/chat/steer', () => true],
  ['/stats', () => (state.empty ? fx.emptyStats : fx.stats(14))],
  ['/providers', () => state.providers],
  ['/providers/models', () => ['claude-sonnet-4-5', 'claude-haiku-4-5']],
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

function route(path: string, body: unknown, query: URLSearchParams): unknown {
  const hit = routes.find(([pattern]) => pattern === path)
  if (hit) return hit[1](body, query)

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
    const s = state.skills.find((x) => x.id === id)
    if (s) s.enabled = Boolean((body as { enabled?: boolean })?.enabled)
    return true
  }
  if (tail === 'content') {
    const s = state.skills.find((x) => x.id === id)
    return { content: `## 什么时候用\n\n${s?.description || ''}\n\n## 怎么做\n\n1. 读取相关资料\n2. 按模板整理\n3. 交给用户确认` }
  }
  if (tail === 'default') return true
  if (tail === 'test') return { ok: true, model: 'claude-sonnet-4-5', detail: '连接正常' }
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
      data: json(route(url.pathname, body, url.searchParams)),
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
