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
  nodeExe,
  nodeVersion,
  npmRun,
  piEntry,
  piInstalled,
  piPrefix,
  piVersion,
  pythonExe,
  pythonVersion,
  gitVersion,
} from './engine/runtime'
import type { AppRowVO } from '../domain/types_core'

const PI_PACKAGE = '@earendil-works/pi-coding-agent'

let busy_kind = ''

/** 是否有安装动作在跑。 */
export function appBusy(kind?: string): boolean {
  return busy_kind !== '' && (!kind || busy_kind === kind)
}

function progress(phase: 'start' | 'line' | 'done' | 'fail', line?: string, message?: string): void {
  broadcast('app:progress', { kind: 'pi', action: busy_action, phase, line, message, engine_kind: 'pi' })
}

let busy_action = 'install'

/** 应用清单：pi 本体 + 三个内置运行时。 */
export async function appsList(): Promise<{ apps: AppRowVO[] }> {
  const p = paths()
  const node_ready = fs.existsSync(nodeExe())
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
  const runtimes: [string, string, string, () => Promise<string>, string][] = [
    ['Node.js', nodeExe(), p.wb_node_dir, nodeVersion, 'agent 运行时与包管理'],
    ['PortableGit', gitExe(), p.wb_git_dir, gitVersion, 'bash / git 工具链'],
    ['Python', pythonExe(), p.wb_python_dir, pythonVersion, 'python 脚本运行时'],
  ]
  for (const [name, exe, dir, probe, note] of runtimes) {
    const installed = fs.existsSync(exe.includes('\\') ? exe : '') || fs.existsSync(dir)
    rows.push({
      kind: 'pi',
      app_name: name,
      installed,
      version: installed ? await probe() : '',
      exe: installed ? exe : '',
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
  if (!fs.existsSync(nodeExe())) {
    throw appError(CODE_RUNTIME_UNPACK, '内置 Node.js 尚未就绪，稍等片刻再试')
  }
  if (appBusy('pi')) throw appError(CODE_PROVISION, '已有安装任务在进行')
  busy_kind = 'pi'
  busy_action = 'install'
  broadcast('engine:status')
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
  } finally {
    busy_kind = ''
    broadcast('engine:status')
    broadcast('engine:ready')
  }
  return appsList()
}

/** 卸载 pi。 */
export async function appUninstall(_engine: string): Promise<{ apps: AppRowVO[] }> {
  if (appBusy('pi')) throw appError(CODE_PROVISION, '已有安装任务在进行')
  busy_kind = 'pi'
  busy_action = 'uninstall'
  try {
    progress('start', undefined, '正在卸载 pi')
    await npmRun(['uninstall', '--prefix', piPrefix(), PI_PACKAGE, '--no-audit', '--no-fund'], (line) =>
      progress('line', line),
    )
    fs.rmSync(piPrefix(), { recursive: true, force: true })
    progress('done', undefined, 'pi 已卸载')
    logInfo('pi 已卸载')
  } finally {
    busy_kind = ''
    broadcast('engine:status')
  }
  return appsList()
}

/** bash / git 路径给其他服务用（避免重复导出）。 */
export const runtimePaths = { bashExe }
