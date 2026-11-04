//! 引擎与应用路由：engines / health / apps（pi 与内置运行时的安装面）。
import type { Router } from 'express'
import { ok, route } from '../pkg/resp'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_PROVISION } from '../domain/errors'
import { enginesDetect, enginesRestart, enginesStop, engineStatus, healthView } from '../service/bootstrap_service'
import { appInstall, appUninstall, appsList } from '../service/app_service'

export function registerEngineRoutes(api: Router): void {
  api.get('/engines', route(async (_req, res) => {
    ok(res, engineStatus())
  }))

  api.post('/engines/detect', route(async (_req, res) => {
    ok(res, await enginesDetect())
  }))

  api.post('/engines/restart', route(async (req, res) => {
    ok(res, await enginesRestart(req.body?.engine))
  }))

  api.post('/engines/stop', route(async (req, res) => {
    ok(res, await enginesStop(String(req.body?.engine ?? 'pi'), String(req.body?.work_dir ?? '')))
  }))

  api.get('/health/engines', route(async (_req, res) => {
    ok(res, await healthView())
  }))

  api.get('/apps', route(async (_req, res) => {
    ok(res, await appsList())
  }))

  api.post('/apps/install', route(async (req, res) => {
    ok(res, await runAppAction('install', String(req.body?.engine ?? '')))
  }))

  api.post('/apps/update', route(async (req, res) => {
    ok(res, await runAppAction('update', String(req.body?.engine ?? '')))
  }))

  api.post('/apps/uninstall', route(async (req, res) => {
    ok(res, await runAppAction('uninstall', String(req.body?.engine ?? '')))
  }))
}

/** 应用动作只支持引擎本体；运行时行不可操作。 */
async function runAppAction(action: 'install' | 'update' | 'uninstall', engine: string) {
  if (engine !== 'pi') throw appError(CODE_PROVISION, `不支持对 ${engine || '该目标'} 执行 ${action}`)
  try {
    if (action === 'uninstall') return await appUninstall(engine)
    return await appInstall(engine)
  } catch (err) {
    throw wrapError(CODE_PROVISION, `${action} 失败`, err)
  }
}
