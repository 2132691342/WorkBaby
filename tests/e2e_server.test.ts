//! 端到端长链路：起真 HTTP 服务，验鉴权 → 路由 → service → SQLite → 信封全链。
import { beforeAll, describe, expect, it, vi } from 'vitest'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

// electron 只用 app / shell 的少量能力，测试里给桩。
vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getVersion: () => '0.0.0-test',
    getLoginItemSettings: () => ({ openAtLogin: false }),
    setLoginItemSettings: () => undefined,
  },
  shell: { openPath: async () => '', showItemInFolder: () => undefined },
}))

let token = ''
let base = ''

beforeAll(async () => {
  // 数据根重定向到临时目录，避免写进真实 %LOCALAPPDATA%。
  process.env.LOCALAPPDATA = fs.mkdtempSync(path.join(os.tmpdir(), 'wb-e2e-'))
  const { initContext } = await import('../electron/main/context')
  const { paths } = await import('../electron/main/context')
  const { openDb } = await import('../electron/main/repo/db')
  const { buildServer, startServer } = await import('../electron/main/api/server')
  const { token: ctxToken } = await import('../electron/main/context')
  initContext()
  openDb(paths().wb_db_path)
  const port = await startServer(buildServer())
  token = ctxToken()
  base = `http://127.0.0.1:${port}/api/v1`
})

async function call(method: 'GET' | 'POST', url: string, body?: unknown, with_token = true): Promise<{ status: number; json: Record<string, unknown> }> {
  const res = await fetch(`${base}${url}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(with_token ? { 'X-Desktop-Token': token } : {}),
    },
    body: method === 'POST' ? JSON.stringify(body ?? {}) : undefined,
  })
  return { status: res.status, json: (await res.json()) as Record<string, unknown> }
}

describe('HTTP 全链路', () => {
  it('令牌鉴权：无令牌与错令牌都拿不到数据', async () => {
    const none = await call('GET', '/bootstrap', undefined, false)
    expect(none.status).toBe(401)
    const res = await fetch(`${base}/bootstrap`, { headers: { 'X-Desktop-Token': 'fake' } })
    expect(res.status).toBe(401)
  })

  it('bootstrap 信封与引擎身份', async () => {
    const { json } = await call('GET', '/bootstrap')
    expect(json.code).toBe('0')
    const data = json.data as Record<string, unknown>
    expect(data.active).toBe('pi')
    const engines = data.engines as { kind: string }[]
    expect(engines).toHaveLength(1)
    expect(engines[0].kind).toBe('pi')
  })

  it('设置写读往返', async () => {
    const saved = await call('POST', '/settings/save', { 'ui.theme': 'dark' })
    expect(saved.json.code).toBe('0')
    const read = await call('GET', '/settings')
    expect((read.json.data as Record<string, string>)['ui.theme']).toBe('dark')
  })

  it('MCP 保存 → 清单可见 → 开关关闭', async () => {
    const saved = await call('POST', '/mcp/save', {
      server_name: 'fs',
      command_text: 'npx -y @modelcontextprotocol/server-filesystem "C:/x"',
      desc: '文件服务',
    })
    expect(saved.json.code).toBe('0')
    const list = await call('GET', '/mcp')
    const servers = (list.json.data as { servers: { server_name: string; command_line?: string; enabled: boolean }[] }).servers
    const row = servers.find((s) => s.server_name === 'fs')
    expect(row?.command_line).toContain('server-filesystem')
    expect(row?.enabled).toBe(true)
    const off = await call('POST', '/mcp/toggle', { server_name: 'fs', enabled: false })
    const after = ((off.json.data as { servers: { server_name: string; enabled: boolean }[] }).servers).find(
      (s) => s.server_name === 'fs',
    )
    expect(after?.enabled).toBe(false)
  })

  it('技能新建 → 清单可见 → 删除', async () => {
    const created = await call('POST', '/skills/create', {
      name: 'e2e-skill',
      description: '端到端用例造的技能',
      body: '用法说明',
    })
    expect(created.json.code).toBe('0')
    const list = await call('GET', '/skills')
    const skills = (list.json.data as { skills: { name: string }[] }).skills
    expect(skills.some((s) => s.name === 'e2e-skill')).toBe(true)
    const file = (created.json.data as { path: string }).path
    const removed = await call('POST', '/skills/delete', { path: file })
    expect(removed.json.code).toBe('0')
  })

  it('未知接口与工作区越界都回业务码', async () => {
    const missing = await call('GET', '/no-such-api')
    expect(missing.json.code).toBe('1404')
    const escape = await call('GET', '/files/read?path=../../etc/passwd')
    expect(escape.json.code).not.toBe('0')
  })
})
