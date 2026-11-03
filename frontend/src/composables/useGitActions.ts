// 工作区页的 git 写动作：暂存 / 取消暂存 / 丢弃 / 提交 / 分支 / 同步。
// 全部收口在 act()：写动作共享「挂 loading → 重拉状态与差异 → 失败给原话」，
// 成功也给提示（否则「拉取」没动静分不清拉完了还是没点中）；不可逆动作先确认。
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { GitFile, GitStatus } from '@/types/api'

/**
 * 丢弃改动 / 强推 / 中止这类动作的确认。
 *
 * 话术与按钮文字由调用方给：确认框要复述「将发生什么」，
 * 而那句话只有调用方知道（丢弃的是哪个文件、中止的是哪次合并）。
 */
async function confirmDanger(
  body: string,
  title: string,
  okText: string,
  cancelText: string,
): Promise<boolean> {
  try {
    await ElMessageBox.confirm(body, title, {
      confirmButtonText: okText,
      cancelButtonText: cancelText,
      type: 'warning',
    })
    return true
  } catch {
    return false
  }
}

/**
 * 绑定一个「当前状态 + 刷新动作」。
 *
 * pending 暴露给界面挂 loading（按钮靠它禁用与转圈），
 * 刷新回调由调用方给——它还要同时刷新分支与历史，那些不在本文件里。
 */
export function useGitActions(
  status: () => GitStatus | null,
  reload: () => Promise<void>,
) {
  const { t } = useI18n()
  const toasts = useToasts()
  const pending = ref('')

  /**
   * 跑一个 git 写动作，并给出一句结果提示。
   *
   * done 是成功话术；为空表示这个动作成功也不必打扰用户
   * （例如批量暂存：列表自己变了，用户看得见）。
   *
   * 成功也要说话：拉取这类动作没有可见副作用时，「点下去没动静」与
   * 「压根没点中」在界面上长得一模一样。
   */
  async function act(
    name: string,
    fn: () => Promise<GitStatus | void>,
    done = '',
  ): Promise<void> {
    pending.value = name
    try {
      await fn()
      // 动作自己的返回值不直接写进 status：它只是这一次的响应，
      // 而界面上还有分支、历史、差异要看——统一重拉才不会只更新一半。
      await reload()
      if (done) toasts.push('ok', done)
    } catch (err) {
      toasts.push('error', err instanceof Error ? err.message : String(err))
    } finally {
      pending.value = ''
    }
  }

  /** 唯一远端时直接用它；有多个或没有就留空，让后端挑默认。 */
  function soleRemote(): string {
    const remotes = status()?.remotes ?? []
    return remotes.length === 1 ? remotes[0].remote_name : ''
  }

  async function stageAll(files: GitFile[]): Promise<void> {
    await act(
      'stage',
      () => api.gitStage(files.map((f) => f.file_path)),
      t('workspace.stagedAll', { n: files.length }),
    )
  }

  async function stage(file: GitFile): Promise<void> {
    await act(
      `stage:${file.file_path}`,
      () => api.gitStage([file.file_path]),
      t('workspace.stagedOne', { path: file.file_path }),
    )
  }

  async function unstage(file: GitFile): Promise<void> {
    await act(
      `unstage:${file.file_path}`,
      () => api.gitUnstage([file.file_path]),
      t('workspace.unstagedOne', { path: file.file_path }),
    )
  }

  async function discard(file: GitFile): Promise<void> {
    const ok = await confirmDanger(
      t('workspace.discardConfirm', { path: file.file_path }),
      t('workspace.discardTitle'),
      t('workspace.discard'),
      t('common.cancel'),
    )
    if (!ok) return
    await act(
      `discard:${file.file_path}`,
      () => api.gitDiscard([file.file_path]),
      t('workspace.discarded', { path: file.file_path }),
    )
  }

  async function resolveConflicts(files: GitFile[]): Promise<void> {
    await act(
      'resolve',
      () => api.gitResolve(files.map((f) => f.file_path)),
      t('workspace.resolved', { n: files.length }),
    )
  }

  /** 标记单个文件已解决（冲突时逐个处理比一批更常见）。 */
  async function resolve(file: GitFile): Promise<void> {
    await act(
      `resolve:${file.file_path}`,
      () => api.gitResolve([file.file_path]),
      t('workspace.resolvedOne', { path: file.file_path }),
    )
  }

  async function stateAction(action: 'continue' | 'abort'): Promise<void> {
    if (action === 'abort') {
      const ok = await confirmDanger(
        t('workspace.abortConfirm'),
        t('workspace.abortTitle'),
        t('workspace.abort'),
        t('common.cancel'),
      )
      if (!ok) return
    }
    await act(
      `state:${action}`,
      () => api.gitState(action),
      action === 'abort' ? t('workspace.aborted') : t('workspace.continued'),
    )
  }

  async function initRepo(): Promise<void> {
    await act('init', () => api.gitInit('main'), t('workspace.initDone'))
  }

  async function switchBranch(name: string): Promise<void> {
    await act(`switch:${name}`, () => api.gitSwitchBranch(name), t('workspace.switchedBranch', { name }))
  }

  async function createBranch(): Promise<void> {
    let name = ''
    try {
      const result = await ElMessageBox.prompt(t('workspace.branchName'), t('workspace.newBranch'), {
        confirmButtonText: t('common.save'),
        cancelButtonText: t('common.cancel'),
      })
      name = result.value
    } catch {
      return // 用户取消
    }
    await act(`branch`, () => api.gitCreateBranch(name), t('workspace.createdBranch', { name }))
  }

  /**
   * 推送。
   *
   * force 分三档：none 普通、lease 带租约、force 无条件。
   * 无条件强推会静默覆盖别人的提交，所以要用户输入分支名确认。
   */
  async function push(force = 'none'): Promise<void> {
    const branch = status()?.branch_name ?? ''
    let confirm = ''
    if (force === 'force') {
      try {
        const { value } = await ElMessageBox.prompt(
          t('workspace.forceConfirmPlaceholder', { name: branch }),
          t('workspace.forcePush'),
          { confirmButtonText: t('workspace.forcePush'), cancelButtonText: t('common.cancel') },
        )
        confirm = value
      } catch {
        return
      }
    }
    await act(
      `push:${force}`,
      () => api.gitPush(soleRemote(), branch, force, !status()?.upstream_ref, confirm),
      t('workspace.pushed', { branch }),
    )
  }

  async function pull(): Promise<void> {
    const branch = status()?.branch_name ?? ''
    // 拉完说清「有没有新东西」：git pull 成功但无事发生时，
    // 不说一声会被当成失败——用户看不出区别。
    const before = status()?.behind ?? 0
    await act(
      'pull',
      () => api.gitPull(soleRemote(), branch, false),
      before > 0
        ? t('workspace.pulledN', { branch, n: before })
        : t('workspace.pulledUpToDate', { branch }),
    )
  }

  async function fetchRemote(): Promise<void> {
    await act('fetch', () => api.gitFetch(soleRemote()), t('workspace.fetched'))
  }

  /**
   * 删除分支。
   *
   * 只删本地分支，且必须先确认：删掉的分支名要靠 reflog 才能找回来，
   * 而这是界面上最容易误点的一个动作。
   */
  async function deleteBranch(name: string): Promise<void> {
    const ok = await confirmDanger(
      t('workspace.deleteBranchConfirm', { name }),
      t('workspace.deleteBranch'),
      t('common.delete'),
      t('common.cancel'),
    )
    if (!ok) return
    await act(`delete:${name}`, () => api.gitDeleteBranch(name), t('workspace.deletedBranch', { name }))
  }

  /** 合并一个分支进当前分支。 */
  async function mergeBranch(name: string): Promise<void> {
    await act(`merge:${name}`, () => api.gitMerge(name, false), t('workspace.merged', { name }))
  }

  /** 添加远端地址。没有远端时同步类按钮全部不渲染，这里是唯一的出口。 */
  async function addRemote(): Promise<void> {
    let url = ''
    try {
      const result = await ElMessageBox.prompt(t('workspace.remoteUrl'), t('workspace.addRemote'), {
        confirmButtonText: t('common.save'),
        cancelButtonText: t('common.cancel'),
        inputPlaceholder: 'https://github.com/user/repo.git',
      })
      url = result.value.trim()
    } catch {
      return
    }
    if (!url) return
    await act('remote', () => api.gitAddRemote('origin', url), t('workspace.remoteAdded'))
  }

  return {
    pending,
    stageAll,
    stage,
    unstage,
    discard,
    resolve,
    resolveConflicts,
    stateAction,
    initRepo,
    switchBranch,
    createBranch,
    deleteBranch,
    mergeBranch,
    addRemote,
    push,
    pull,
    fetchRemote,
  }
}
