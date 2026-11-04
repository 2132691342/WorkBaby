//! 应用与设置路由：bootstrap / settings / prefs / config / logs / system / docs。
import type { Router } from 'express'
import { shell } from 'electron'
import { ok, route } from '../pkg/resp'
import { appError } from '../pkg/apperror'
import { CODE_PATH_MISSING } from '../domain/errors'
import { broadcast } from '../sse/hub'
import { bootstrapView } from '../service/bootstrap_service'
import {
  settingsAll,
  settingsSave,
  workspaceSet,
  workspaceForget,
  prefsGet,
  prefsSetAutoStart,
  prefsSetRestore,
  prefsSetCloseAction,
  prefsRememberSession,
  prefsOpenDataDir,
  configFiles,
  configText,
  logsRecent,
} from '../service/settings_service'
import { chatOnWorkspaceChanged } from '../service/chat/chat_service'
import { docsList, docsContent } from '../service/docs_service'
import { getWorkspaceHistory } from '../repo/settings_repo'

export function registerAppRoutes(api: Router): void {
  api.get('/bootstrap', route(async (_req, res) => {
    ok(res, bootstrapView())
  }))

  api.get('/settings', route(async (_req, res) => {
    ok(res, settingsAll())
  }))

  api.post('/settings/save', route(async (req, res) => {
    const kv = (req.body ?? {}) as Record<string, string>
    ok(res, settingsSave(kv))
  }))

  api.post('/settings/workspace', route(async (req, res) => {
    const dir = String(req.body?.path ?? '').trim()
    if (!dir) throw appError(CODE_PATH_MISSING, '工作区路径不能为空')
    const workspace = workspaceSet(dir)
    chatOnWorkspaceChanged()
    broadcast('workspace:changed', { workspace })
    ok(res, { workspace })
  }))

  api.post('/settings/workspace/forget', route(async (req, res) => {
    const dir = String(req.body?.path ?? '')
    ok(res, { workspaces: workspaceForget(dir) })
  }))

  api.post('/settings/engine', route(async (req, res) => {
    // 只有一个引擎；参数是什么都落到 pi。
    ok(res, { engine: 'pi' })
  }))

  api.get('/logs', route(async (_req, res) => {
    ok(res, logsRecent())
  }))

  api.get('/prefs', route(async (_req, res) => {
    ok(res, prefsGet())
  }))

  api.post('/prefs/autostart', route(async (req, res) => {
    ok(res, prefsSetAutoStart(req.body?.enabled === true))
  }))

  api.post('/prefs/restore', route(async (req, res) => {
    ok(res, prefsSetRestore(req.body?.enabled === true))
  }))

  api.post('/prefs/close-action', route(async (req, res) => {
    const action = req.body?.action === 'quit' ? 'quit' : 'tray'
    ok(res, prefsSetCloseAction(action))
  }))

  api.post('/prefs/session', route(async (req, res) => {
    ok(res, prefsRememberSession(String(req.body?.engine_kind ?? 'pi'), String(req.body?.session_file ?? '')))
  }))

  api.post('/prefs/data-dir', route(async (_req, res) => {
    await prefsOpenDataDir()
    ok(res, null)
  }))

  api.get('/config/files', route(async (_req, res) => {
    ok(res, configFiles())
  }))

  api.get('/config/text', route(async (req, res) => {
    ok(res, configText(String(req.query.file ?? '')))
  }))

  api.post('/system/reveal', route(async (req, res) => {
    const p = String(req.body?.path ?? '')
    if (p) shell.showItemInFolder(p)
    ok(res, { ok: true })
  }))

  api.post('/system/open', route(async (req, res) => {
    const p = String(req.body?.path ?? '')
    if (p) await shell.openPath(p)
    ok(res, { ok: true })
  }))

  api.get('/docs', route(async (_req, res) => {
    ok(res, docsList())
  }))

  api.get('/docs/:name', route(async (req, res) => {
    ok(res, docsContent(String(req.params.name ?? '')))
  }))

  // 工作区历史清单（bootstrap 已带，独立端点给下拉刷新用）。
  api.get('/workspaces', route(async (_req, res) => {
    ok(res, { workspaces: getWorkspaceHistory() })
  }))
}
