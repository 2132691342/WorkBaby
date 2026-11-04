//! 应用管理：内置运行时与 pi 本体的安装、升级、卸载与状态行。
import fs from 'node:fs'
import { paths } from '../context'
import { logInfo, logWarn } from '../pkg/log'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_PROVISION, CODE_RUNTIME_UNPACK } from '../domain/errors'
import { broadcast } from '../sse/hub'
import {
  bashExe,
  gitExe,
  gitReady,
  gitVersion,
  nodeExe,
  nodeReady,
  nodeVersion,
  npmRun,
  piEntry,
  piInstalled,
  piPrefix,
  piVersion,
  pythonExe,
  pythonReady,
  pythonVersion,
} from './engine/runtime'
import { provisionBuiltinPlugins } from './plugin_service'
import type { AppRowVO } from '../domain/types_core'

const PI_PACKAGE = '@earendil-works/pi-coding-agent'

let busy_kind = ''
let busy_action = 'install'

/** 是否有安装动作在跑。 */
export function appBusy(kind?: string): boolean {
  return busy_kind !== '' && (!kind || busy_kind === kind)
}

/** 占住安装位；调用方必须在 finally 里 endBusy。 */
export function beginBusy(kind: string, action: string): void {
  busy_kind = kind
  busy_action = action
  broadcast('engine:status')
}

/** 放开安装位并广播状态。 */
export function endBusy(): void {
  busy_kind = ''
  broadcast('engine:status')
}

/** 当前忙碌动作（进度载荷用）。 */
export function appBusyAction(): string {
  return busy_action
}

function progress(phase: 'start' | 'line' | 'done' | 'fail', line?: string, message?: string): void {
  broadcast('app:progress', { kind: 'pi', action: busy_action, phase, line, message, engine_kind: 'pi' })
}

/** 应用清单：pi 本体 + 三个内置运行时。 */
export async function appsList(): Promise<{ apps: AppRowVO[] }> {
  const p = paths()
  const node_ready = nodeReady()
  const pi_ok = piInstalled()
  const rows: AppRowVO[] = []
  rows.push({
    kind: 'pi',
    app_name: 'pi coding agent',
    installed: pi_ok,
    version: pi_ok ? piVersion() : '',
    exe: pi_ok ? piEntry() : '',
    source: 'wb-installer',
    channel: 'wb-installer',
    channel_label: '内置安装器',
    latest_version: '',
    outdated: false,
    install_command: `npm install ${PI_PACKAGE}`,
    update_command: `npm install ${PI_PACKAGE}@latest`,
    uninstall_command: `npm uninstall ${PI_PACKAGE}`,
    docs_url: 'https://github.com/earendil-works/pi',
    can_install: node_ready && !appBusy('pi'),
    can_update: pi_ok && !appBusy('pi'),
    can_uninstall: pi_ok && !appBusy('pi'),
    blocker: node_ready ? undefined : '内置 Node.js 尚未就绪',
    busy: appBusy('pi'),
  })
  // 三个运行时的行：未就绪时不拿路径去探测，避免把「没装」读成「版本为空」。
  const runtimes: [string, boolean, string, string, () => Promise<string>, string][] = [
    ['Node.js', node_ready, nodeExe(), p.wb_node_dir, nodeVersion, 'agent 运行时与包管理'],
    ['PortableGit', gitReady(), gitExe(), p.wb_git_dir, gitVersion, 'bash / git 工具链'],
    ['Python', pythonReady(), pythonExe(), p.wb_python_dir, pythonVersion, 'python 脚本运行时'],
  ]
  for (const [name, ready, exe, dir, probe, note] of runtimes) {
    rows.push({
      kind: 'pi',
      app_name: name,
      installed: ready,
      version: ready ? await probe() : '',
      exe: ready ? exe : '',
      source: 'builtin',
      channel: 'wb-installer',
      channel_label: '内置运行时',
      latest_version: '',
      outdated: false,
      install_command: '首次启动自动解压',
      update_command: '',
      uninstall_command: '',
      docs_url: '',
      can_install: false,
      can_update: false,
      can_uninstall: false,
      busy: false,
      update_preview: note,
    })
  }
  return { apps: rows }
}

/** 安装或升级 pi：用内置 npm 拉最新版，逐行广播进度。 */
export async function appInstall(_engine: string): Promise<{ apps: AppRowVO[] }> {
  if (!nodeReady()) {
    throw appError(CODE_RUNTIME_UNPACK, '内置 Node.js 尚未就绪，稍等片刻再试')
  }
  if (appBusy('pi')) throw appError(CODE_PROVISION, '已有安装任务在进行')
  beginBusy('pi', 'install')
  try {
    progress('start', undefined, '正在安装 pi coding agent')
    const args = ['install', '--prefix', piPrefix(), `${PI_PACKAGE}@latest`, '--no-audit', '--no-fund']
    const r = await npmRun(args, (line) => progress('line', line))
    if (r.code !== 0) {
      logWarn(`pi 安装失败：${r.stderr.slice(0, 500)}`)
      throw wrapError(CODE_PROVISION, 'pi 安装失败', new Error(r.stderr.slice(0, 2000)))
    }
    progress('done', undefined, `pi ${piVersion()} 安装完成`)
    logInfo(`pi 安装完成：${piVersion()}`)
    // 装完 pi 立刻把内置插件补上：用户不需要再跑一次「安装插件」。
    const plugin_result = await provisionBuiltinPlugins()
    progress('done', undefined, `内置插件就绪 ${plugin_result.installed} 个`)
  } finally {
    endBusy()
    broadcast('engine:ready')
  }
  return appsList()
}

/** 卸载 pi。 */
export async function appUninstall(_engine: string): Promise<{ apps: AppRowVO[] }> {
  if (appBusy('pi')) throw appError(CODE_PROVISION, '有安装任务正在进行')
  beginBusy('pi', 'uninstall')
  try {
    progress('start', undefined, '正在卸载 pi')
    await npmRun(['uninstall', '--prefix', piPrefix(), PI_PACKAGE, '--no-audit', '--no-fund'], (line) =>
      progress('line', line),
    )
    fs.rmSync(piPrefix(), { recursive: true, force: true })
    progress('done', undefined, 'pi 已卸载')
    logInfo('pi 已卸载')
  } finally {
    endBusy()
  }
  return appsList()
}

/** bash / git 路径给其他服务用（避免重复导出）。 */
export const runtimePaths = { bashExe }
