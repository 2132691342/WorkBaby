//! 数据路由：会话清单 / 详情 / 删除 / 改名，文件浏览与 git 面板。
import type { Router } from 'express'
import { ok, route } from '../pkg/resp'
import { sessionDelete, sessionDetail, sessionRename, sessionsList } from '../service/session_service'
import { currentWorkspace } from '../service/settings_service'
import { fileList, fileRead, fileSearch } from '../service/file_service'
import {
  gitStatus,
  gitDiff,
  gitCachedDiff,
  gitHistory,
  gitCommitDetail,
  gitFileHistory,
  gitFileDiff,
  gitBranches,
  gitStage,
  gitUnstage,
  gitDiscard,
  gitResolve,
  gitCommit,
  gitCreateBranch,
  gitSwitch,
  gitDeleteBranch,
  gitMerge,
  gitStateAction,
  gitInit,
  gitAddRemote,
  gitPush,
  gitPull,
  gitFetch,
} from '../service/git_service'

function asPaths(raw: unknown): string[] {
  return Array.isArray(raw) ? raw.map(String) : []
}

export function registerDataRoutes(api: Router): void {
  api.get('/sessions', route(async (req, res) => {
    const workspace = req.query.workspace ? String(req.query.workspace) : currentWorkspace()
    ok(res, sessionsList(workspace || undefined))
  }))

  api.get('/sessions/detail', route(async (req, res) => {
    ok(res, sessionDetail(String(req.query.path ?? '')))
  }))

  api.post('/sessions/delete', route(async (req, res) => {
    ok(res, sessionDelete(String(req.body?.path ?? '')))
  }))

  api.post('/sessions/rename', route(async (req, res) => {
    ok(res, sessionRename(String(req.body?.path ?? ''), String(req.body?.name ?? '')))
  }))

  api.get('/files', route(async (req, res) => {
    ok(res, fileList(String(req.query.path ?? '')))
  }))

  api.get('/files/search', route(async (req, res) => {
    const limit = Number(req.query.limit ?? 50) || 50
    ok(res, fileSearch(String(req.query.query ?? ''), limit))
  }))

  api.get('/files/read', route(async (req, res) => {
    ok(res, fileRead(String(req.query.path ?? '')))
  }))

  api.get('/git/status', route(async (_req, res) => {
    ok(res, await gitStatus())
  }))

  api.get('/git/diff', route(async (req, res) => {
    ok(res, await gitDiff(req.query.path ? String(req.query.path) : undefined))
  }))

  api.get('/git/cached-diff', route(async (req, res) => {
    ok(res, await gitCachedDiff(req.query.path ? String(req.query.path) : undefined))
  }))

  api.get('/git/history', route(async (req, res) => {
    ok(res, await gitHistory(Number(req.query.limit ?? 50) || 50))
  }))

  api.get('/git/commit', route(async (req, res) => {
    ok(res, await gitCommitDetail(String(req.query.hash ?? '')))
  }))

  api.get('/git/file-history', route(async (req, res) => {
    ok(res, await gitFileHistory(String(req.query.path ?? ''), Number(req.query.limit ?? 40) || 40))
  }))

  api.get('/git/file-diff', route(async (req, res) => {
    ok(res, await gitFileDiff(String(req.query.hash ?? ''), String(req.query.path ?? '')))
  }))

  api.get('/git/branches', route(async (_req, res) => {
    ok(res, await gitBranches())
  }))

  api.post('/git/stage', route(async (req, res) => {
    ok(res, await gitStage(asPaths(req.body?.paths)))
  }))

  api.post('/git/unstage', route(async (req, res) => {
    ok(res, await gitUnstage(asPaths(req.body?.paths)))
  }))

  api.post('/git/discard', route(async (req, res) => {
    ok(res, await gitDiscard(asPaths(req.body?.paths)))
  }))

  api.post('/git/resolve', route(async (req, res) => {
    ok(res, await gitResolve(asPaths(req.body?.paths)))
  }))

  api.post('/git/commit', route(async (req, res) => {
    ok(res, await gitCommit(String(req.body?.message ?? ''), req.body?.amend === true))
  }))

  api.post('/git/branch', route(async (req, res) => {
    ok(res, await gitCreateBranch(String(req.body?.name ?? '')))
  }))

  api.post('/git/switch', route(async (req, res) => {
    ok(res, await gitSwitch(String(req.body?.name ?? '')))
  }))

  api.post('/git/branch/delete', route(async (req, res) => {
    ok(res, await gitDeleteBranch(String(req.body?.name ?? '')))
  }))

  api.post('/git/merge', route(async (req, res) => {
    ok(res, await gitMerge(String(req.body?.name ?? ''), req.body?.rebase === true))
  }))

  api.post('/git/state', route(async (req, res) => {
    const action = req.body?.action === 'continue' ? 'continue' : 'abort'
    ok(res, await gitStateAction(action))
  }))

  api.post('/git/init', route(async (req, res) => {
    ok(res, await gitInit(String(req.body?.branch ?? '')))
  }))

  api.post('/git/remote', route(async (req, res) => {
    ok(res, await gitAddRemote(String(req.body?.name ?? ''), String(req.body?.url ?? '')))
  }))

  api.post('/git/push', route(async (req, res) => {
    ok(
      res,
      await gitPush(
        String(req.body?.remote_name ?? 'origin'),
        String(req.body?.branch_name ?? ''),
        String(req.body?.force ?? 'none'),
        req.body?.set_upstream === true,
      ),
    )
  }))

  api.post('/git/pull', route(async (req, res) => {
    ok(
      res,
      await gitPull(String(req.body?.remote_name ?? 'origin'), String(req.body?.branch_name ?? ''), req.body?.rebase === true),
    )
  }))

  api.post('/git/fetch', route(async (req, res) => {
    ok(res, await gitFetch(String(req.body?.remote_name ?? '')))
  }))
}
