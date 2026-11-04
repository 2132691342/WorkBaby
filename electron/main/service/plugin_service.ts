//! 插件管理：npm 装进沙箱目录，扩展桥接文件放进 pi 的 extensions，开关靠移动目录。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import { readJson, writeJson, readText } from '../pkg/fsutil'
import { logInfo, logWarn } from '../pkg/log'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_PROVISION } from '../domain/errors'
import { npmRun } from './engine/runtime'
import { appBusy } from './app_service'
import { broadcast } from '../sse/hub'
import type { PluginListDataVO, PluginRowVO } from '../domain/types_data'

/** 注册表：slug → 来源与开关意图。 */
interface PluginRecord {
  plugin_source: string
  plugin_name: string
  plugin_version: string
  installed_at: number
  enabled: boolean
}

type Registry = Record<string, PluginRecord>

function pluginsRoot(): string {
  return path.join(paths().wb_pi_agent_dir, 'plugins')
}

function extensionsDir(): string {
  return path.join(paths().wb_pi_agent_dir, 'extensions')
}

function registryPath(): string {
  return path.join(paths().wb_pi_agent_dir, 'plugin-registry.json')
}

function loadRegistry(): Registry {
  return readJson<Registry>(registryPath()) ?? {}
}

function saveRegistry(reg: Registry): void {
  writeJson(registryPath(), reg)
}

/** 从 npm 来源推出包名与目录别名：npm:@s/p@1.2 → @s/p + s-p。 */
function parseSource(source: string): { pkg: string; slug: string } {
  let raw = source.trim()
  raw = raw.replace(/^npm:/, '')
  if (raw.startsWith('git:')) raw = raw.slice(4)
  const pkg = raw.replace(/@[^@]+$/, '').replace(/\/$/, '') || raw
  const slug = pkg.replace(/^@/, '').replace(/[^A-Za-z0-9._-]+/g, '-').toLowerCase()
  return { pkg, slug }
}

/** 在 <dir>/node_modules 下找包真实目录；作用域包多看一层。 */
function findPkgDir(dir: string, pkg: string): string | null {
  const nm = path.join(dir, 'node_modules')
  const direct = path.join(nm, pkg)
  if (fs.existsSync(path.join(direct, 'package.json'))) return direct
  const queue = [nm]
  while (queue.length > 0) {
    const cur = queue.shift() as string
    let names: string[] = []
    try {
      names = fs.readdirSync(cur)
    } catch {
      continue
    }
    for (const name of names) {
      const full = path.join(cur, name)
      let stat: fs.Stats
      try {
        stat = fs.statSync(full)
      } catch {
        continue
      }
      if (!stat.isDirectory() || name === '.bin') continue
      if (fs.existsSync(path.join(full, 'package.json'))) {
        const meta = readJson<{ name?: string }>(path.join(full, 'package.json'))
        if (meta?.name === pkg) return full
      } else {
        queue.push(full)
      }
    }
  }
  return null
}

/** 解析扩展入口：优先包内 pi.extensions 清单，其次 main / dist / src 约定。 */
function resolveEntries(pkg_dir: string): string[] {
  const meta = readJson<{ pi?: { extensions?: string[] }; main?: string }>(path.join(pkg_dir, 'package.json'))
  const declared = meta?.pi?.extensions
  if (declared?.length) {
    return declared.map((rel) => path.resolve(pkg_dir, rel)).filter((p) => fs.existsSync(p))
  }
  const candidates = [meta?.main, 'dist/index.js', 'index.js', 'src/index.ts'].filter(Boolean) as string[]
  for (const rel of candidates) {
    const full = path.resolve(pkg_dir, rel)
    if (fs.existsSync(full)) return [full]
  }
  return []
}

/** 写桥接目录：<extensions>/<slug>/package.json 指向真实入口。 */
function writeShim(slug: string, entries: string[]): string {
  const shim = path.join(extensionsDir(), slug)
  fs.rmSync(shim, { recursive: true, force: true })
  fs.mkdirSync(shim, { recursive: true })
  writeJson(path.join(shim, 'package.json'), { name: slug, pi: { extensions: entries } })
  return shim
}

function progress(action: string, phase: 'start' | 'line' | 'done' | 'fail', line?: string, message?: string): void {
  broadcast('app:progress', { kind: 'pi', action, phase, line, message, engine_kind: 'pi' })
}

/** 装一个插件：npm 安装 → 写桥接 → 记注册表。 */
export async function pluginInstall(source: string): Promise<{ output: string }> {
  if (!source.trim()) throw appError(CODE_PROVISION, '插件来源不能为空')
  if (appBusy('pi')) throw appError(CODE_PROVISION, '已有安装任务在进行')
  const { pkg, slug } = parseSource(source)
  const dir = path.join(pluginsRoot(), slug)
  fs.rmSync(dir, { recursive: true, force: true })
  fs.mkdirSync(dir, { recursive: true })
  progress('install', 'start', undefined, `正在安装 ${pkg}`)
  try {
    const r = await npmRun(['install', '--prefix', dir, source, '--no-audit', '--no-fund'], (line) =>
      progress('install', 'line', line),
    )
    if (r.code !== 0) throw wrapError(CODE_PROVISION, `插件安装失败：${pkg}`, new Error(r.stderr.slice(0, 2000)))
    const pkg_dir = findPkgDir(dir, pkg)
    if (!pkg_dir) throw appError(CODE_PROVISION, `安装完成但找不到包目录：${pkg}`)
    const entries = resolveEntries(pkg_dir)
    if (entries.length === 0) throw appError(CODE_PROVISION, `包 ${pkg} 不是 pi 扩展（找不到入口）`)
    const meta = readJson<{ name?: string; version?: string }>(path.join(pkg_dir, 'package.json'))
    writeShim(slug, entries)
    const reg = loadRegistry()
    reg[slug] = {
      plugin_source: source,
      plugin_name: meta?.name ?? pkg,
      plugin_version: meta?.version ?? '',
      installed_at: Date.now(),
      enabled: true,
    }
    saveRegistry(reg)
    progress('install', 'done', undefined, `${pkg}@${meta?.version ?? ''} 安装完成`)
    logInfo(`插件安装完成：${pkg}`)
    return { output: `${pkg}@${meta?.version ?? ''} 安装完成` }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    progress('install', 'fail', undefined, message)
    throw err
  }
}

/** 卸载插件：清目录、清桥接、清注册表。 */
export async function pluginUninstall(source: string): Promise<{ output: string }> {
  const reg = loadRegistry()
  const slug = matchSlug(reg, source)
  if (!slug) throw appError(CODE_PROVISION, `未找到插件：${source}`)
  const { pkg } = parseSource(reg[slug].plugin_source)
  fs.rmSync(path.join(pluginsRoot(), slug), { recursive: true, force: true })
  fs.rmSync(path.join(extensionsDir(), slug), { recursive: true, force: true })
  fs.rmSync(path.join(extensionsDir(), `.disabled-${slug}`), { recursive: true, force: true })
  delete reg[slug]
  saveRegistry(reg)
  logInfo(`插件已卸载：${pkg}`)
  return { output: `${pkg} 已卸载` }
}

/** 开关：桥接目录改名，pi 自然发现 / 不发现。 */
export async function pluginToggle(plugin_name: string, enabled: boolean): Promise<{ output: string }> {
  const reg = loadRegistry()
  const slug = matchSlug(reg, plugin_name) ?? plugin_name
  const shim = path.join(extensionsDir(), slug)
  const hidden = path.join(extensionsDir(), `.disabled-${slug}`)
  if (enabled && fs.existsSync(hidden)) {
    fs.renameSync(hidden, shim)
  } else if (!enabled && fs.existsSync(shim)) {
    fs.renameSync(shim, hidden)
  }
  if (reg[slug]) {
    reg[slug].enabled = enabled
    saveRegistry(reg)
  }
  logInfo(`插件${enabled ? '启用' : '停用'}：${plugin_name}`)
  return { output: `${plugin_name} 已${enabled ? '启用' : '停用'}` }
}

/** 升级：按注册来源重装一遍。 */
export async function pluginUpgrade(plugin_source: string): Promise<{ output: string }> {
  const reg = loadRegistry()
  const slug = matchSlug(reg, plugin_source)
  if (!slug) throw appError(CODE_PROVISION, `未找到插件：${plugin_source}`)
  const spec = reg[slug].plugin_source.replace(/@[^@]+$/, '') || plugin_source
  await pluginInstall(spec)
  return { output: `${spec} 已升级` }
}

function matchSlug(reg: Registry, key: string): string | null {
  if (reg[key]) return key
  for (const [slug, rec] of Object.entries(reg)) {
    if (rec.plugin_name === key || rec.plugin_source === key || slug === parseSource(key).slug) return slug
  }
  return null
}

/** 插件清单：注册表 + 磁盘实况合并。 */
export function pluginList(): PluginListDataVO {
  const reg = loadRegistry()
  const rows: PluginRowVO[] = []
  for (const [slug, rec] of Object.entries(reg)) {
    const shim = path.join(extensionsDir(), slug)
    const hidden = path.join(extensionsDir(), `.disabled-${slug}`)
    const pkg_dir = path.join(pluginsRoot(), slug, 'node_modules')
    const meta = readJson<{ description?: string }>(findPkgJson(pkg_dir, rec.plugin_name) ?? '')
    rows.push({
      engine_kind: 'pi',
      plugin_name: rec.plugin_name,
      plugin_version: rec.plugin_version,
      plugin_source: rec.plugin_source,
      plugin_path: fs.existsSync(shim) || fs.existsSync(hidden) ? shim : '',
      plugin_desc: meta?.description ?? '',
      resource_kinds: ['extension'],
      scope_kind: 'user',
      enabled: fs.existsSync(shim) && rec.enabled,
      installed: fs.existsSync(path.join(pluginsRoot(), slug)),
    })
  }
  return { plugins: rows, registry_path: registryPath() }
}

function findPkgJson(nm: string, pkg: string): string | null {
  const direct = path.join(nm, ...pkg.split('/'), 'package.json')
  if (fs.existsSync(direct)) return direct
  return null
}

/** 桥接目录里现在真正生效的扩展（自省用）。 */
export function activeExtensionCount(): number {
  let count = 0
  try {
    count = fs
      .readdirSync(extensionsDir(), { withFileTypes: true })
      .filter((d) => d.isDirectory() && !d.name.startsWith('.')).length
  } catch {
    count = 0
  }
  return count
}

/** 读插件说明文件（面板展示用，读不到给空串）。 */
export function pluginReadme(slug: string): string {
  return readText(path.join(pluginsRoot(), slug, 'README.md')) ?? ''
}
