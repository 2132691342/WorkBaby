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
    const data = list.json.data as { skills: { name: string }[]; builtin_total: number }
    expect(data.skills.some((s) => s.name === 'e2e-skill')).toBe(true)
    // 内置清单计数必须真的来自清单，不是拍脑袋的常量
    expect(data.builtin_total).toBeGreaterThan(0)
    const file = (created.json.data as { path: string }).path
    const removed = await call('POST', '/skills/delete', { path: file })
    expect(removed.json.code).toBe('0')
  })

  it('插件清单带出 12 个内置项，且声明真实写进了 settings.json', async () => {
    // 首启的写盘动作由 bootstrap 触发；测试环境里先跑一次写接口模拟它。
    await call('POST', '/plugins/toggle', { plugin_name: 'pi-web-access', enabled: true })

    const list = await call('GET', '/plugins')
    expect(list.json.code).toBe('0')
    const data = list.json.data as {
      plugins: { plugin_name: string; builtin: boolean; plugin_state: string }[]
      builtin_total: number
      registry_path: string
      install_root: string
    }
    expect(data.builtin_total).toBe(12)
    expect(data.plugins.filter((p) => p.builtin)).toHaveLength(12)
    // 沙箱还没解压运行时：要么未装（missing），要么已装，绝不能是别的态
    for (const row of data.plugins) {
      expect(['installed', 'missing', 'failed']).toContain(row.plugin_state)
    }
    expect(data.registry_path.endsWith('settings.json')).toBe(true)
    expect(data.install_root.endsWith(path.join('pi', 'npm'))).toBe(true)

    const declared = fs.readFileSync(data.registry_path, 'utf8')
    for (const name of ['pi-web-access', '@juicesharp/rpiv-todo', 'context-mode']) {
      expect(declared, `settings.json 应声明 ${name}`).toContain(name)
    }
  })

  it('插件开关只改声明：关→开 后条目回到裸来源串', async () => {
    // 这一条验的是「声明」而不是行上的 enabled：包没装时行恒为未启用（enabled = 声明 && 已装），
    // 所以直接读 settings.json 才是真事实。
    const off = await call('POST', '/plugins/toggle', { plugin_name: 'pi-web-access', enabled: false })
    expect(off.json.code).toBe('0')
    const list = await call('GET', '/plugins')
    const registry = (list.json.data as { registry_path: string }).registry_path
    const afterOff = JSON.parse(fs.readFileSync(registry, 'utf8')) as {
      packages: (string | { source: string; autoload?: boolean })[]
    }
    const entryOff = afterOff.packages.find((p) =>
      (typeof p === 'string' ? p : p.source).includes('pi-web-access'),
    )
    expect(entryOff, 'pi-web-access 应在 packages 里').toBeTruthy()
    expect(typeof entryOff).toBe('object')
    expect((entryOff as { autoload?: boolean }).autoload).toBe(false)

    const on = await call('POST', '/plugins/toggle', { plugin_name: 'pi-web-access', enabled: true })
    expect(on.json.code).toBe('0')
    const afterOn = JSON.parse(fs.readFileSync(registry, 'utf8')) as {
      packages: (string | { source: string; autoload?: boolean })[]
    }
    const entryOn = afterOn.packages.find((p) =>
      (typeof p === 'string' ? p : p.source).includes('pi-web-access'),
    )
    // 启用态回到裸来源串：pi 的 packages 就是这么表达的
    expect(typeof entryOn).toBe('string')
    expect(entryOn).toBe('npm:pi-web-access@0.42.0')
  })

  it('未声明的插件开关回业务码而不是静默成功', async () => {
    // 先保证内置清单已声明，这样下面被测的是「非内置且未声明」这一条路径
    await call('POST', '/plugins/toggle', { plugin_name: 'pi-web-access', enabled: true })
    const res = await call('POST', '/plugins/toggle', { plugin_name: 'not-a-real-plugin', enabled: true })
    expect(res.json.code).not.toBe('0')
    expect(String(res.json.message)).toContain('not-a-real-plugin')
  })

  it('未知接口与工作区越界都回业务码', async () => {
    const missing = await call('GET', '/no-such-api')
    expect(missing.json.code).toBe('1404')
    const escape = await call('GET', '/files/read?path=../../etc/passwd')
    expect(escape.json.code).not.toBe('0')
  })
})
