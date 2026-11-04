//! 内置运行时：三个 zip 解压落位 + node/npm/git/python/pi 的路径与版本探测。
import fs from 'node:fs'
import path from 'node:path'
import AdmZip from 'adm-zip'
import { paths } from '../../context'
import { ensureDir, readJson } from '../../pkg/fsutil'
import { logInfo, logWarn } from '../../pkg/log'
import { broadcast } from '../../sse/hub'
import { runProc } from '../../pkg/proc'
import { toolEnv } from '../../pkg/env'
import { appError } from '../../pkg/apperror'
import { CODE_GIT_UNAVAILABLE, CODE_RUNTIME_UNPACK } from '../../domain/errors'

/** 解压一个 zip 到目标目录；zip 内单一根目录时把它改名成目标名。 */
function unpackZip(zip_path: string, target: string): void {
  const tmp = `${target}.unpack`
  fs.rmSync(tmp, { recursive: true, force: true })
  ensureDir(tmp)
  const zip = new AdmZip(zip_path)
  zip.extractAllTo(tmp, true)
  const names = fs.readdirSync(tmp)
  if (names.length === 1 && fs.statSync(path.join(tmp, names[0])).isDirectory()) {
    fs.renameSync(path.join(tmp, names[0]), target)
  } else {
    fs.renameSync(tmp, target)
  }
  fs.rmSync(tmp, { recursive: true, force: true })
}

function progress(phase: 'start' | 'line' | 'done' | 'fail', message?: string): void {
  broadcast('app:progress', {
    kind: 'pi',
    action: 'install',
    phase,
    message,
    engine_kind: 'pi',
  })
}

/** 检查运行时是否齐备；缺哪个解哪个，全部就绪后广播状态。 */
export async function provisionRuntime(): Promise<void> {
  const p = paths()
  const items: { zip: string; target: string; label: string }[] = [
    { zip: 'node-v24.21.0-win-x64.zip', target: p.wb_node_dir, label: 'Node.js' },
    { zip: 'PortableGit.zip', target: p.wb_git_dir, label: 'PortableGit' },
    { zip: 'Python313.zip', target: p.wb_python_dir, label: 'Python' },
  ]
  const assets = path.join(p.wb_assets_dir, 'runtime')
  for (const item of items) {
    if (fs.existsSync(item.target)) continue
    const zip_path = path.join(assets, item.zip)
    if (!fs.existsSync(zip_path)) {
      logWarn(`运行时压缩包缺失：${zip_path}`)
      continue
    }
    progress('start', `正在准备内置 ${item.label}`)
    try {
      ensureDir(p.wb_runtime_dir)
      unpackZip(zip_path, item.target)
      progress('done', `内置 ${item.label} 就绪`)
      logInfo(`运行时解压完成：${item.label}`)
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      logWarn(`运行时解压失败 ${item.label}：${raw}`)
      progress('fail', `内置 ${item.label} 解压失败：${raw}`)
    }
  }
  broadcast('engine:status')
}

/**
 * 内置 node.exe；沙箱里没有就报错。
 * 绝不回退系统 node——那等于壳在拿用户环境跑包，恰恰是本壳要避免的事。
 */
export function nodeExe(): string {
  const local = path.join(paths().wb_node_dir, 'node.exe')
  if (fs.existsSync(local)) return local
  throw appError(CODE_RUNTIME_UNPACK, '内置 Node.js 尚未就绪，稍等片刻再试')
}

/** 内置 node.exe 是否存在（不抛的探测版，给状态行用）。 */
export function nodeReady(): boolean {
  return fs.existsSync(path.join(paths().wb_node_dir, 'node.exe'))
}

/** 内置 npm-cli.js。 */
export function npmCli(): string {
  return path.join(paths().wb_node_dir, 'node_modules', 'npm', 'bin', 'npm-cli.js')
}

/** 内置 git.exe；沙箱里没有就报错，不回退系统 git。 */
export function gitExe(): string {
  const local = path.join(paths().wb_git_dir, 'cmd', 'git.exe')
  if (fs.existsSync(local)) return local
  throw appError(CODE_GIT_UNAVAILABLE, '内置 PortableGit 尚未就绪，稍等片刻再试')
}

/** 内置 git.exe 是否存在（不抛的探测版）。 */
export function gitReady(): boolean {
  return fs.existsSync(path.join(paths().wb_git_dir, 'cmd', 'git.exe'))
}

/** 内置 bash.exe（agent 主力 shell）。 */
export function bashExe(): string {
  return path.join(paths().wb_git_dir, 'bin', 'bash.exe')
}

/** 内置 python.exe；沙箱里没有就报错，不回退系统 python。 */
export function pythonExe(): string {
  const local = path.join(paths().wb_python_dir, 'python.exe')
  if (fs.existsSync(local)) return local
  throw appError(CODE_RUNTIME_UNPACK, '内置 Python 尚未就绪，稍等片刻再试')
}

/** 内置 python.exe 是否存在（不抛的探测版）。 */
export function pythonReady(): boolean {
  return fs.existsSync(path.join(paths().wb_python_dir, 'python.exe'))
}

/** pi 安装前缀目录。 */
export function piPrefix(): string {
  return path.join(paths().wb_data_dir, 'pi-app')
}

/** pi 包目录（npm --prefix 安装产物）。 */
export function piPkgDir(): string {
  return path.join(piPrefix(), 'node_modules', '@earendil-works', 'pi-coding-agent')
}

/** pi CLI 入口。 */
export function piEntry(): string {
  return path.join(piPkgDir(), 'dist', 'bundle', 'cli.js')
}

/** pi 是否已安装。 */
export function piInstalled(): boolean {
  return fs.existsSync(piEntry())
}

/** pi 版本：读它的 package.json。 */
export function piVersion(): string {
  const pkg = readJson<{ version?: string }>(path.join(piPkgDir(), 'package.json'))
  return pkg?.version ?? ''
}

/** node 版本；探测失败给空串。 */
export async function nodeVersion(): Promise<string> {
  try {
    const r = await runProc(nodeExe(), ['--version'], { timeout_ms: 10_000 })
    return r.proc_stdout.trim()
  } catch {
    return ''
  }
}

/** git 版本；探测失败给空串。 */
export async function gitVersion(): Promise<string> {
  try {
    const r = await runProc(gitExe(), ['--version'], { timeout_ms: 10_000 })
    return r.proc_stdout.trim()
  } catch {
    return ''
  }
}

/** python 版本；探测失败给空串。 */
export async function pythonVersion(): Promise<string> {
  try {
    const r = await runProc(pythonExe(), ['--version'], { timeout_ms: 10_000 })
    return r.proc_stderr.trim() || r.proc_stdout.trim()
  } catch {
    return ''
  }
}

/** 用内置 npm 跑一条安装命令；逐行经 onLine 上报进度。 */
export async function npmRun(
  args: string[],
  onLine?: (line: string) => void,
): Promise<{ code: number; stderr: string }> {
  const r = await runProc(nodeExe(), [npmCli(), ...args], {
    cwd: paths().wb_data_dir,
    env: toolEnv(),
    timeout_ms: 10 * 60_000,
    on_line: onLine,
  })
  return { code: r.proc_code, stderr: r.proc_stderr }
}
