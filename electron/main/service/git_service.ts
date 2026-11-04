//! git 面板：状态 / 差异 / 历史全部经内置 git；写操作限白名单动作。
import fs from 'node:fs'
import path from 'node:path'
import { currentWorkspace } from './settings_service'
import { gitExe, gitVersion } from './engine/runtime'
import { runProc } from '../pkg/proc'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_GIT_FAILED, CODE_GIT_UNAVAILABLE, CODE_PATH_MISSING } from '../domain/errors'
import { parseDiffLines } from '../domain/types_data'
import type {
  GitBranchVO,
  GitCommitDetailVO,
  GitCommitResultVO,
  GitCommitVO,
  GitDiffVO,
  GitFileVO,
  GitStatusVO,
} from '../domain/types_data'
import type { GitChange } from '../domain/types_data'

const DIFF_MAX = 600_000

interface GitRun {
  code: number
  out: string
  err: string
}

/** 跑一条 git 命令；工作区未设时报路径缺失。 */
async function git(args: string[], cwd?: string): Promise<GitRun> {
  const dir = cwd ?? currentWorkspace()
  if (!dir) throw appError(CODE_PATH_MISSING, '请先选择工作区')
  const r = await runProc(gitExe(), args, { cwd: dir, timeout_ms: 120_000 }).catch((err: unknown) => {
    throw wrapError(CODE_GIT_UNAVAILABLE, 'git 不可用（内置运行时未就绪）', err)
  })
  return { code: r.proc_code, out: r.proc_stdout, err: r.proc_stderr }
}

/** 组装 git 状态视图。 */
export async function gitStatus(): Promise<GitStatusVO> {
  const ws = currentWorkspace()
  const base: GitStatusVO = {
    available: false,
    is_repo: false,
    workspace: ws,
    repo_root: '',
    branch_name: '',
    upstream_ref: '',
    ahead: 0,
    behind: 0,
    repo_state: 'clean',
    repo_state_detail: '',
    remotes: [],
    changed_files: [],
    conflict: false,
    git_version: '',
  }
  if (!ws) return { ...base, unavailable_reason: '未选择工作区' }
  let version = ''
  try {
    version = await gitVersion()
  } catch {
    /* 探测失败不阻塞后续 */
  }
  base.git_version = version
  if (!version) return { ...base, unavailable_reason: 'git 不可用' }
  const root = await git(['rev-parse', '--show-toplevel']).catch(() => null)
  if (!root || root.code !== 0) {
    return { ...base, available: true, is_repo: false }
  }
  base.available = true
  base.is_repo = true
  base.repo_root = root.out.trim()

  const [porcelain, numstatU, numstatS, remotes, state] = await Promise.all([
    git(['status', '--porcelain=v2', '--branch']),
    git(['diff', '--numstat']),
    git(['diff', '--cached', '--numstat']),
    git(['remote', '-v']),
    Promise.resolve(repoState(base.repo_root)),
  ])
  parsePorcelain(porcelain.out, base)
  applyNumstat(base, numstatU.out, false)
  applyNumstat(base, numstatS.out, true)
  for (const line of remotes.out.split('\n')) {
    const m = /^(\S+)\s+(\S+)/.exec(line.trim())
    if (m) base.remotes.push({ remote_name: m[1], remote_url: m[2] })
  }
  base.repo_state = state.state
  base.repo_state_detail = state.detail
  return base
}

/** porcelain v2 → 分支与变更文件。 */
function parsePorcelain(text: string, base: GitStatusVO): void {
  for (const line of text.split('\n')) {
    if (line.startsWith('# branch.head ')) {
      base.branch_name = line.slice(14).trim()
    } else if (line.startsWith('# branch.upstream ')) {
      base.upstream_ref = line.slice(18).trim()
    } else if (line.startsWith('# branch.ab ')) {
      const m = /\+(\d+) -(\d+)/.exec(line)
      if (m) {
        base.ahead = Number(m[1])
        base.behind = Number(m[2])
      }
    } else if (line.startsWith('1 ')) {
      // 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>：路径可能带空格，取第 8 段之后整段。
      const parts = line.split(' ')
      const xy = parts[1]
      const file_path = parts.slice(8).join(' ')
      push(base, file_path, xy[0], false)
    } else if (line.startsWith('2 ')) {
      const rest = line.slice(2)
      const tab = rest.indexOf('\t')
      const meta = rest.slice(0, tab)
      const paths = rest.slice(tab + 1)
      const parts = meta.split(' ')
      const xy = parts[0]
      const from = paths.split('\t')[1] ?? ''
      push(base, from, xy[0], false)
    } else if (line.startsWith('u ')) {
      const rest = line.slice(2)
      const tab = rest.indexOf('\t')
      const file_path = rest.slice(tab + 1).split('\t').pop() ?? ''
      push(base, file_path, 'U', false)
    } else if (line.startsWith('? ')) {
      push(base, line.slice(2).trim(), '?', false)
    }
  }
}

function push(base: GitStatusVO, file_path: string, x: string, staged: boolean): void {
  const code = (x === '?' ? '?' : x) as GitChange
  base.changed_files.push({
    file_path: file_path.replace(/"/g, ''),
    change_code: code,
    staged: staged || (x !== ' ' && x !== '?'),
    conflict: code === 'U',
    additions: 0,
    deletions: 0,
  })
  if (code === 'U') base.conflict = true
}

function applyNumstat(base: GitStatusVO, text: string, staged: boolean): void {
  for (const line of text.split('\n')) {
    const m = /^(\d+|-)\t(\d+|-)\t(.+)$/.exec(line.trim())
    if (!m) continue
    const row = base.changed_files.find((f) => f.file_path === m[3] && f.staged === staged)
    if (row) {
      row.additions = m[1] === '-' ? 0 : Number(m[1])
      row.deletions = m[2] === '-' ? 0 : Number(m[2])
    }
  }
}

/** 检测仓库进行中的操作状态。 */
function repoState(root: string): { state: string; detail: string } {
  const dot = path.join(root, '.git')
  const checks: [string, string][] = [
    [path.join(dot, 'MERGE_HEAD'), 'merge'],
    [path.join(dot, 'rebase-merge'), 'rebase'],
    [path.join(dot, 'rebase-apply'), 'rebase'],
    [path.join(dot, 'CHERRY_PICK_HEAD'), 'cherry-pick'],
    [path.join(dot, 'REVERT_HEAD'), 'revert'],
    [path.join(dot, 'BISECT_LOG'), 'bisect'],
  ]
  for (const [file, name] of checks) {
    try {
      if (fs.existsSync(file)) return { state: name, detail: `仓库处于 ${name} 状态` }
    } catch {
      /* 忽略 */
    }
  }
  return { state: 'clean', detail: '' }
}

/** 工作区差异（未暂存或指定文件）。 */
export async function gitDiff(file?: string): Promise<GitDiffVO> {
  const args = ['diff', '--no-color', '-U3']
  if (file) args.push('--', file)
  const r = await git(args)
  return buildDiff(r.out, file ?? '工作区')
}

/** 暂存区差异。 */
export async function gitCachedDiff(file?: string): Promise<GitDiffVO> {
  const args = ['diff', '--cached', '--no-color', '-U3']
  if (file) args.push('--', file)
  const r = await git(args)
  return buildDiff(r.out, file ?? '暂存区')
}

function buildDiff(raw: string, label: string): GitDiffVO {
  const truncated = raw.length > DIFF_MAX
  const text = truncated ? raw.slice(0, DIFF_MAX) : raw
  return { file_path: label, truncated, dropped: 0, diff_lines: parseDiffLines(text) }
}

/** 提交历史。 */
export async function gitHistory(limit = 50): Promise<GitCommitVO[]> {
  const r = await git(['log', `--max-count=${limit}`, '--pretty=format:%H%x1f%h%x1f%s%x1f%an%x1f%aI'])
  return r.out
    .split('\n')
    .filter(Boolean)
    .map((line) => {
      const [hash, short, subject, author, time] = line.split('\x1f')
      return {
        commit_hash: hash,
        short_hash: short,
        subject,
        author_name: author,
        commit_time: time,
        file_count: 0,
        additions: 0,
        deletions: 0,
      }
    })
}

/** 单个提交详情：元数据 + 变更文件 + 差异。 */
export async function gitCommitDetail(hash: string): Promise<GitCommitDetailVO> {
  // 元数据段与差异段用 \x1e 分开，父提交行挂在段尾。
  const r = await git(['show', '--no-color', '-U3', '--pretty=format:%H%x1f%h%x1f%s%x1f%an%x1f%aI%x1f%b%x1e%P', hash])
  const sep = r.out.indexOf('\x1e')
  const meta = sep >= 0 ? r.out.slice(0, sep) : r.out
  const diff_raw = sep >= 0 ? r.out.slice(sep + 1) : ''
  const [hash1, short, subject, author, time, body] = meta.split('\x1f')
  const parents = (sep >= 0 ? r.out.slice(sep + 1).split('\n')[0] : '').trim()
  const diff_text = parents ? diff_raw.split('\n').slice(1).join('\n') : diff_raw
  const files: GitFileVO[] = []
  for (const line of diff_text.split('\n')) {
    const m = /^(\d+|-)\t(\d+|-)\t(.+)$/.exec(line.trim())
    if (m) {
      files.push({
        file_path: m[3],
        change_code: 'M',
        staged: true,
        conflict: false,
        additions: m[1] === '-' ? 0 : Number(m[1]),
        deletions: m[2] === '-' ? 0 : Number(m[2]),
      })
    }
  }
  return {
    commit_hash: hash1 ?? hash,
    short_hash: short ?? hash.slice(0, 7),
    subject: subject ?? '',
    author_name: author ?? '',
    commit_time: time ?? '',
    file_count: files.length,
    additions: files.reduce((s, f) => s + f.additions, 0),
    deletions: files.reduce((s, f) => s + f.deletions, 0),
    body: body ?? '',
    parent_hash: parents.split(' ').filter(Boolean),
    changed_file: files,
    diff: buildDiff(diff_text, hash),
  }
}

/** 文件历史。 */
export async function gitFileHistory(file: string, limit = 40): Promise<{ commits: GitCommitVO[] }> {
  const r = await git(['log', `--max-count=${limit}`, '--pretty=format:%H%x1f%h%x1f%s%x1f%an%x1f%aI', '--', file])
  return {
    commits: r.out
      .split('\n')
      .filter(Boolean)
      .map((line) => {
        const [hash, short, subject, author, time] = line.split('\x1f')
        return {
          commit_hash: hash,
          short_hash: short,
          subject,
          author_name: author,
          commit_time: time,
          file_count: 1,
          additions: 0,
          deletions: 0,
        }
      }),
  }
}

/** 某个提交里某文件的差异。 */
export async function gitFileDiff(hash: string, file: string): Promise<GitDiffVO> {
  const r = await git(['show', '--no-color', '-U3', hash, '--', file])
  return buildDiff(r.out, file)
}

/** 分支清单。 */
export async function gitBranches(): Promise<GitBranchVO[]> {
  const r = await git(['branch', '-vv', '--no-color'])
  return r.out
    .split('\n')
    .filter((l) => l.trim())
    .map((line) => {
      const current = line.startsWith('*')
      const rest = line.slice(2).trim()
      const [name, ...info] = rest.split(/\s+/)
      const upstream = info.join(' ')
      const m = /\[([^\]:]+)(?:: ahead (\d+))?(?:, behind (\d+))?\]/.exec(upstream)
      return {
        branch_name: name,
        current,
        remote_name: m?.[1] ?? '',
        ahead: Number(m?.[2] ?? 0),
        behind: Number(m?.[3] ?? 0),
      }
    })
}

async function runOk(args: string[], what: string): Promise<void> {
  const r = await git(args)
  if (r.code !== 0) throw wrapError(CODE_GIT_FAILED, `${what}失败`, new Error(r.err || r.out))
}

/** 暂存 / 取消暂存 / 丢弃。 */
export async function gitStage(paths: string[]): Promise<GitStatusVO> {
  await runOk(['add', '--', ...paths], '暂存')
  return gitStatus()
}

export async function gitUnstage(paths: string[]): Promise<GitStatusVO> {
  await runOk(['reset', 'HEAD', '--', ...paths], '取消暂存')
  return gitStatus()
}

export async function gitDiscard(paths: string[]): Promise<GitStatusVO> {
  await runOk(['checkout', '--', ...paths], '丢弃')
  return gitStatus()
}

/** 标记冲突已解决：等价于把文件加入暂存。 */
export async function gitResolve(paths: string[]): Promise<GitStatusVO> {
  await runOk(['add', '--', ...paths], '标记已解决')
  return gitStatus()
}

/** 提交。 */
export async function gitCommit(message: string, amend = false): Promise<GitCommitResultVO> {
  if (!message.trim()) throw appError(CODE_GIT_FAILED, '提交信息不能为空')
  const args = amend ? ['commit', '--amend', '-m', message] : ['commit', '-m', message]
  await runOk(args, '提交')
  const r = await git(['rev-parse', 'HEAD'])
  return { status: 'ok', commit_hash: r.out.trim() }
}

/** 新建分支。 */
export async function gitCreateBranch(name: string): Promise<GitStatusVO> {
  await runOk(['branch', name], '新建分支')
  return gitStatus()
}

/** 切换分支。 */
export async function gitSwitch(name: string): Promise<GitStatusVO> {
  await runOk(['checkout', name], '切换分支')
  return gitStatus()
}

/** 删除分支。 */
export async function gitDeleteBranch(name: string): Promise<GitStatusVO> {
  await runOk(['branch', '-D', name], '删除分支')
  return gitStatus()
}

/** 合并；可选 rebase。 */
export async function gitMerge(name: string, rebase: boolean): Promise<GitStatusVO> {
  await runOk(rebase ? ['rebase', name] : ['merge', name], rebase ? '变基' : '合并')
  return gitStatus()
}

/** 冲突的继续 / 放弃。 */
export async function gitStateAction(action: 'continue' | 'abort'): Promise<GitStatusVO> {
  const st = await gitStatus()
  if (st.repo_state === 'merge') {
    await runOk(action === 'continue' ? ['merge', '--continue'] : ['merge', '--abort'], '合并状态')
  } else if (st.repo_state.startsWith('rebase')) {
    await runOk(action === 'continue' ? ['rebase', '--continue'] : ['rebase', '--abort'], '变基状态')
  } else if (st.repo_state === 'cherry-pick') {
    await runOk(action === 'continue' ? ['cherry-pick', '--continue'] : ['cherry-pick', '--abort'], '摘取状态')
  } else if (st.repo_state === 'revert') {
    await runOk(action === 'continue' ? ['revert', '--continue'] : ['revert', '--abort'], '回退状态')
  }
  return gitStatus()
}

/** 初始化仓库。 */
export async function gitInit(branch: string): Promise<GitStatusVO> {
  const args = ['init']
  if (branch) args.push('-b', branch)
  await runOk(args, '初始化仓库')
  return gitStatus()
}

/** 添加远端。 */
export async function gitAddRemote(name: string, url: string): Promise<GitStatusVO> {
  await runOk(['remote', 'add', name, url], '添加远端')
  return gitStatus()
}

/** 推送；force: none | lease | force。 */
export async function gitPush(remote: string, branch: string, force: string, setUpstream: boolean): Promise<GitStatusVO> {
  const args = ['push']
  if (force === 'lease') args.push('--force-with-lease')
  else if (force === 'force') args.push('--force')
  if (setUpstream) args.push('-u')
  await runOk([...args, remote, branch], '推送')
  return gitStatus()
}

/** 拉取。 */
export async function gitPull(remote: string, branch: string, rebase: boolean): Promise<GitStatusVO> {
  await runOk(['pull', ...(rebase ? ['--rebase'] : []), remote, branch], '拉取')
  return gitStatus()
}

/** 抓取。 */
export async function gitFetch(remote: string): Promise<GitStatusVO> {
  await runOk(['fetch', ...(remote ? [remote] : [])], '抓取')
  return gitStatus()
}
