//! pi 的 settings.json 编排：packages 声明、启停过滤与 npmCommand 指向内置 npm。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import { readJson, writeJson } from '../pkg/fsutil'
import { nodeExe, nodeReady, npmCli } from './engine/runtime'

/** settings.json 里 packages 的条目：字符串或带过滤的对象两种形态。 */
export type PackageEntry = string | { source: string; autoload?: boolean; [k: string]: unknown }

/** 我们关心的 settings.json 字段；其余键原样保留。 */
export interface PiSettings {
  npmCommand?: string[]
  packages?: PackageEntry[]
  [k: string]: unknown
}

function settingsPath(): string {
  return path.join(paths().wb_pi_agent_dir, 'settings.json')
}

/** 读 settings.json；不存在或坏掉给空对象，绝不抛。 */
export function readSettings(): PiSettings {
  return readJson<PiSettings>(settingsPath()) ?? {}
}

function writeSettings(value: PiSettings): void {
  writeJson(settingsPath(), value)
}

/** 从条目取来源串。 */
export function sourceOf(entry: PackageEntry): string {
  return typeof entry === 'string' ? entry : entry.source
}

/** 入口是否处于停用态：对象形态的 autoload === false。 */
export function entryEnabled(entry: PackageEntry): boolean {
  return typeof entry === 'string' || entry.autoload !== false
}

/** `npm:@scope/pkg@1.2.3` → `@scope/pkg`；无作用域同理。 */
export function packageNameOf(source: string): string {
  let raw = source.trim().replace(/^npm:/, '')
  if (raw.startsWith('git:') || raw.startsWith('http')) return raw
  const at = raw.lastIndexOf('@')
  if (at > 0) raw = raw.slice(0, at)
  return raw.replace(/\/$/, '')
}

/**
 * 内置 npm 的调用形状：node + npm-cli.js。
 * pi 的所有包安装都经它，沙箱外的 npm 永远不参与。
 */
export function npmCommandSpec(): string[] {
  return [nodeExe(), npmCli()]
}

/**
 * 把 npmCommand 落到 settings.json。
 * 首启与每次运行时重建后都要调一次——内置 node 路径随数据目录走。
 * 内置 node 还没解压时直接返回：写个半成品路径会让 pi 启动时找不到 npm。
 */
export function ensureNpmCommand(): void {
  if (!nodeReady()) return
  const current = readSettings()
  const want = npmCommandSpec()
  const same = Array.isArray(current.npmCommand) && current.npmCommand.join('\u0000') === want.join('\u0000')
  if (same) return
  writeSettings({ ...current, npmCommand: want })
}

/** 当前 packages 条目（保持声明顺序）。 */
export function listPackages(): PackageEntry[] {
  const value = readSettings().packages
  return Array.isArray(value) ? value.filter((e) => typeof e === 'string' || (e && typeof e === 'object')) : []
}

/**
 * 写入 packages；空数组时删键而不是留 `[]`。
 * 留 `[]` 与「没声明过」在 pi 眼里等价，但界面上看不出差别容易误读成「配了但全停用」。
 */
export function writePackages(entries: PackageEntry[]): void {
  const current = readSettings()
  if (entries.length === 0) delete current.packages
  else current.packages = entries
  writeSettings(current)
}

/** 找出某个包名对应的条目下标；找不到给 -1。 */
export function indexOfPackage(entries: PackageEntry[], plugin_name: string): number {
  return entries.findIndex((e) => packageNameOf(sourceOf(e)) === plugin_name)
}

/**
 * 声明一个包：已存在则原位替换（保住原有位置与顺序），否则追加。
 * enabled=false 时用对象形态的 autoload:false 表达停用。
 */
export function declarePackage(plugin_name: string, source: string, enabled: boolean): PackageEntry[] {
  const entries = listPackages()
  const entry: PackageEntry = enabled ? source : { source, autoload: false }
  const idx = indexOfPackage(entries, plugin_name)
  if (idx >= 0) entries[idx] = entry
  else entries.push(entry)
  writePackages(entries)
  return entries
}

/** 撤掉一个包的声明。 */
export function undeclarePackage(plugin_name: string): PackageEntry[] {
  const entries = listPackages()
  const idx = indexOfPackage(entries, plugin_name)
  if (idx >= 0) entries.splice(idx, 1)
  writePackages(entries)
  return entries
}

/** 改写某个包的启用态；未声明时给 false 表示没改到。 */
export function setPackageEnabled(plugin_name: string, enabled: boolean): boolean {
  const entries = listPackages()
  const idx = indexOfPackage(entries, plugin_name)
  if (idx < 0) return false
  const source = sourceOf(entries[idx])
  entries[idx] = enabled ? source : { source, autoload: false }
  writePackages(entries)
  return true
}

/** pi 的包安装根：agentDir/npm（pi 自己的约定，不要改）。 */
export function pluginInstallRoot(): string {
  return path.join(paths().wb_pi_agent_dir, 'npm')
}

/** 某个 npm 包的安装目录；scope 包多一层。 */
export function installedPathOf(plugin_name: string): string {
  return path.join(pluginInstallRoot(), 'node_modules', ...plugin_name.split('/'))
}

/** 包是否已落到磁盘（以 package.json 为准）。 */
export function isInstalled(plugin_name: string): boolean {
  return fs.existsSync(path.join(installedPathOf(plugin_name), 'package.json'))
}

/** 读已装包的版本与说明；没装给空对象。 */
export function installedMeta(plugin_name: string): { version: string; description: string } {
  const meta = readJson<{ version?: string; description?: string }>(
    path.join(installedPathOf(plugin_name), 'package.json'),
  )
  return { version: meta?.version ?? '', description: meta?.description ?? '' }
}

/** settings.json 绝对路径（界面展示与诊断用）。 */
export function piSettingsPath(): string {
  return settingsPath()
}
