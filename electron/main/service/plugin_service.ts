//! 插件管理：内置清单落 pi 的 packages 声明、经内置 npm 装进沙箱、启停与升级。
import fs from 'node:fs'
import path from 'node:path'
import { readJson } from '../pkg/fsutil'
import { logInfo, logWarn } from '../pkg/log'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_PROVISION } from '../domain/errors'
import { npmRun } from './engine/runtime'
import { appBusy, beginBusy, endBusy } from './app_service'
import { broadcast } from '../sse/hub'
import { BUILTIN_PLUGINS, builtinOf, npmSourceOf } from './plugin_catalog'
import {
  declarePackage,
  entryEnabled,
  indexOfPackage,
  installedMeta,
  installedPathOf,
  isInstalled,
  listPackages,
  packageNameOf,
  piSettingsPath,
  pluginInstallRoot,
  setPackageEnabled,
  sourceOf,
  undeclarePackage,
  writePackages,
} from './plugin_settings'
import type { PluginListDataVO, PluginRowVO, PluginState } from '../domain/types_data'

/** 单个插件的运行期失败原因；进程内有效，重启后由磁盘实况兜底。 */
const failures = new Map<string, string>()

/** 是否把内置清单当「已声明」看；首次运行时写入一次。 */
function ensureBuiltinDeclared(): void {
  const entries = listPackages()
  let changed = false
  for (const item of BUILTIN_PLUGINS) {
    if (indexOfPackage(entries, item.plugin_name) >= 0) continue
    // 内置插件首次声明即按默认开关落 autoload；用户后续开关只改这一个字段。
    const source = npmSourceOf(item)
    if (item.plugin_default_on) entries.push(source)
    else entries.push({ source, autoload: false })
    changed = true
  }
  if (!changed) return
  writePackages(entries)
}

function progress(
  action: string,
  phase: 'start' | 'line' | 'done' | 'fail',
  plugin_name: string,
  line?: string,
  message?: string,
): void {
  broadcast('app:progress', {
    kind: 'pi',
    action,
    phase,
    plugin_name,
    line,
    message,
    engine_kind: 'pi',
  })
}

/** npm 安装参数：与 pi 自己的包安装保持一致（--legacy-peer-deps 避免宿主包被拉副本）。 */
function installArgs(source: string): string[] {
  return ['install', source, '--prefix', pluginInstallRoot(), '--legacy-peer-deps', '--no-audit', '--no-fund']
}

/** 装一个包：npm 落地 → 声明进 settings.json。已装同版本时跳过 npm。 */
async function installOne(source: string, plugin_name: string, action: string): Promise<void> {
  failures.delete(plugin_name)
  const meta = installedMeta(plugin_name)
  const pinned = source.includes('@', 5) ? source.slice(source.lastIndexOf('@') + 1) : ''
  if (meta.version && pinned && meta.version === pinned && isInstalled(plugin_name)) {
    declarePackage(plugin_name, source, true)
    return
  }
  progress(action, 'start', plugin_name, undefined, `正在安装 ${plugin_name}`)
  const r = await npmRun(installArgs(source), (line) => progress(action, 'line', plugin_name, line))
  if (r.code !== 0) {
    const detail = r.stderr.slice(0, 2000) || `npm 退出码 ${r.code}`
    failures.set(plugin_name, detail)
    progress(action, 'fail', plugin_name, undefined, detail.slice(0, 400))
    throw wrapError(CODE_PROVISION, `插件安装失败：${plugin_name}`, new Error(detail))
  }
  if (!isInstalled(plugin_name)) {
    const message = `安装完成但包目录不存在：${plugin_name}`
    failures.set(plugin_name, message)
    progress(action, 'fail', plugin_name, undefined, message)
    throw appError(CODE_PROVISION, message)
  }
  declarePackage(plugin_name, source, true)
  progress(action, 'done', plugin_name, undefined, `${plugin_name}@${installedMeta(plugin_name).version} 就绪`)
  logInfo(`插件安装完成：${plugin_name}@${installedMeta(plugin_name).version}`)
}

/** 首启把内置清单声明落进 settings.json（不装包，装包另走后台）。 */
export function declareBuiltinPlugins(): void {
  ensureBuiltinDeclared()
}

/** 首启装内置插件（后台跑，失败留给界面上单个重试）。 */
export async function provisionBuiltinPlugins(): Promise<{ installed: number; failed: number }> {
  ensureBuiltinDeclared()
  const todo = BUILTIN_PLUGINS.filter((item) => !isInstalled(item.plugin_name))
  if (todo.length === 0) return { installed: 0, failed: 0 }
  logInfo(`内置插件待安装 ${todo.length} 个`)
  let installed = 0
  let failed = 0
  for (const item of todo) {
    try {
      await installOne(npmSourceOf(item), item.plugin_name, 'plugin-install')
      installed += 1
    } catch {
      // 单个失败只记失败表，后面逐个重试。
      failed += 1
    }
    broadcast('plugin:status', { plugin_name: item.plugin_name, engine_kind: 'pi' })
  }
  broadcast('plugin:status', { engine_kind: 'pi' })
  return { installed, failed }
}

/** 界面上的「全部重试」：占住安装位后把缺的包再跑一遍。 */
export async function pluginInstallBuiltin(): Promise<{ installed: number; failed: number }> {
  if (appBusy('pi')) throw appError(CODE_PROVISION, '有安装任务正在进行，稍后再试')
  beginBusy('pi', 'plugin-install')
  try {
    return await provisionBuiltinPlugins()
  } finally {
    endBusy()
  }
}

/** 装一个来源（用户手动加）：来源串可直接 npm: 前缀。 */
export async function pluginInstall(source: string): Promise<{ plugin_name: string }> {
  const raw = source.trim()
  if (!raw) throw appError(CODE_PROVISION, '插件来源不能为空')
  if (appBusy('pi')) throw appError(CODE_PROVISION, '有安装任务正在进行，稍后再试')
  const spec = raw.startsWith('npm:') || raw.startsWith('./') || raw.startsWith('git:') ? raw : `npm:${raw}`
  const plugin_name = packageNameOf(spec)
  if (!plugin_name) throw appError(CODE_PROVISION, '无法从来源解析出包名')
  beginBusy('pi', 'plugin-install')
  try {
    await installOne(spec, plugin_name, 'plugin-install')
  } finally {
    endBusy()
    broadcast('plugin:status', { engine_kind: 'pi' })
  }
  return { plugin_name }
}

/** 重试一个内置插件（界面上的单个重试按钮）。 */
export async function pluginRetry(plugin_name: string): Promise<{ plugin_name: string }> {
  const item = builtinOf(plugin_name)
  if (!item) throw appError(CODE_PROVISION, `不是内置插件：${plugin_name}`)
  if (appBusy('pi')) throw appError(CODE_PROVISION, '有安装任务正在进行，稍后再试')
  beginBusy('pi', 'plugin-install')
  try {
    await installOne(npmSourceOf(item), item.plugin_name, 'plugin-install')
  } finally {
    endBusy()
    broadcast('plugin:status', { engine_kind: 'pi' })
  }
  return { plugin_name }
}

/** 升级：按清单里的 pin 重新装一遍；内置插件回升到内置版本。 */
export async function pluginUpgrade(plugin_name: string): Promise<{ plugin_name: string }> {
  if (appBusy('pi')) throw appError(CODE_PROVISION, '有安装任务正在进行，稍后再试')
  ensureBuiltinDeclared()
  const builtin = builtinOf(plugin_name)
  const entry = listPackages().find((e) => packageNameOf(sourceOf(e)) === plugin_name)
  if (!builtin && !entry) throw appError(CODE_PROVISION, `未找到插件：${plugin_name}`)
  const spec = builtin ? npmSourceOf(builtin) : sourceOf(entry as string | { source: string })
  beginBusy('pi', 'plugin-upgrade')
  try {
    // 先清目录，让 npm 真正重下该版本而非复用残留。
    fs.rmSync(installedPathOf(plugin_name), { recursive: true, force: true })
    await installOne(spec, plugin_name, 'plugin-upgrade')
  } finally {
    endBusy()
    broadcast('plugin:status', { engine_kind: 'pi' })
  }
  return { plugin_name }
}

/** 卸载：撤声明 + 删包目录。内置插件卸载后仍留在清单里（状态回落未安装）。 */
export async function pluginUninstall(plugin_name: string): Promise<{ plugin_name: string }> {
  if (appBusy('pi')) throw appError(CODE_PROVISION, '有安装任务正在进行，稍后再试')
  undeclarePackage(plugin_name)
  failures.delete(plugin_name)
  try {
    fs.rmSync(installedPathOf(plugin_name), { recursive: true, force: true })
  } catch (err) {
    throw wrapError(CODE_PROVISION, `插件目录删除失败：${plugin_name}`, err)
  }
  logInfo(`插件已卸载：${plugin_name}`)
  broadcast('plugin:status', { engine_kind: 'pi' })
  return { plugin_name }
}

/** 开关：只改 settings.json 的 packages 条目，磁盘不动，秒级生效。 */
export function pluginToggle(plugin_name: string, enabled: boolean): { plugin_name: string } {
  // 首次开关前先把内置清单补登：不然未装过 pi 的机器上会拒掉内置项。
  ensureBuiltinDeclared()
  if (!setPackageEnabled(plugin_name, enabled)) {
    throw appError(CODE_PROVISION, `未找到插件声明：${plugin_name}`)
  }
  logInfo(`插件${enabled ? '启用' : '停用'}：${plugin_name}`)
  broadcast('plugin:status', { engine_kind: 'pi' })
  return { plugin_name }
}

/** 包内暴露的资源种类：读 pi 的 manifest，退回惯例目录。 */
function resourceKindsOf(plugin_name: string): string[] {
  const dir = installedPathOf(plugin_name)
  const meta = readJson<{ pi?: Record<string, unknown> }>(path.join(dir, 'package.json'))
  if (meta?.pi) {
    return Object.keys(meta.pi).filter((k) => k !== 'image' && k !== 'video')
  }
  const kinds: string[] = []
  for (const kind of ['extensions', 'skills', 'prompts', 'themes']) {
    if (fs.existsSync(path.join(dir, kind))) kinds.push(kind)
  }
  return kinds.length > 0 ? kinds : ['extensions']
}

/** 单行状态：优先失败表，其次磁盘实况。 */
function stateOf(plugin_name: string): PluginState {
  const error = failures.get(plugin_name)
  if (isInstalled(plugin_name)) return error ? 'failed' : 'installed'
  return error ? 'failed' : 'missing'
}

/** 组装一行。 */
function buildRow(plugin_name: string, source: string, enabled: boolean): PluginRowVO {
  const builtin = builtinOf(plugin_name)
  const meta = installedMeta(source ? plugin_name : '')
  const installed = isInstalled(plugin_name)
  return {
    engine_kind: 'pi',
    plugin_name,
    plugin_version: installed ? meta.version : '',
    plugin_source: source,
    plugin_path: installed ? installedPathOf(plugin_name) : '',
    plugin_desc: builtin?.plugin_desc ?? meta.description,
    resource_kinds: installed ? resourceKindsOf(plugin_name) : [],
    scope_kind: 'user',
    enabled: enabled && installed,
    installed,
    builtin: builtin !== undefined,
    plugin_state: stateOf(plugin_name),
    plugin_error: failures.get(plugin_name),
    plugin_target_version: builtin?.plugin_version ?? '',
  }
}

/**
 * 插件清单 = 内置清单 ∪ settings 里声明的包。
 * 内置项先出（顺序稳定），用户自加的按声明顺序跟在后面。
 * 这里不落盘：只读接口不该改磁盘，声明由首启的 declareBuiltinPlugins 与写操作自己保障。
 */
export function pluginList(): PluginListDataVO {
  const entries = listPackages()
  const rows: PluginRowVO[] = []
  const seen = new Set<string>()
  for (const item of BUILTIN_PLUGINS) {
    const idx = indexOfPackage(entries, item.plugin_name)
    const entry = idx >= 0 ? entries[idx] : npmSourceOf(item)
    rows.push(buildRow(item.plugin_name, sourceOf(entry as string), entryEnabled(entry as string)))
    seen.add(item.plugin_name)
  }
  for (const entry of entries) {
    const source = sourceOf(entry)
    const plugin_name = packageNameOf(source)
    if (!plugin_name || seen.has(plugin_name)) continue
    rows.push(buildRow(plugin_name, source, entryEnabled(entry)))
    seen.add(plugin_name)
  }
  const builtin_installed = rows.filter((r) => r.builtin && r.installed).length
  return {
    plugins: rows,
    registry_path: piSettingsPath(),
    install_root: pluginInstallRoot(),
    builtin_installed,
    builtin_total: BUILTIN_PLUGINS.length,
  }
}

/** 已启用且已装的插件数（健康面板用）。 */
export function pluginCount(): number {
  return pluginList().plugins.filter((p) => p.installed && p.enabled).length
}

/** 清掉失败表（用户在界面上点了「全部重试」时先调）。 */
export function pluginClearFailures(): void {
  failures.clear()
}
