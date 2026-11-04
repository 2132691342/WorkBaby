//! 资源路由：模型 / 服务商 / 登录 / 技能 / 插件 / MCP。
import type { Router } from 'express'
import { ok, route } from '../pkg/resp'
import { modelsList, providerProbe, providerTest, providersView, providerUpsert, providerDelete, loginProviders, loginStart } from '../service/model_service'
import type { ProviderProbeREQ } from '../domain/types_data'
import { skillList, skillContent, skillSave, skillToggle, skillCreate, skillDelete, skillCopy, skillReset } from '../service/skill_service'
import {
  pluginList,
  pluginInstall,
  pluginInstallBuiltin,
  pluginUninstall,
  pluginToggle,
  pluginUpgrade,
  pluginRetry,
  pluginClearFailures,
} from '../service/plugin_service'
import { mcpList, mcpSave, mcpToggle, mcpRemove, mcpCopy } from '../service/mcp_service'
import type { McpSaveREQ } from '../domain/types_data'

export function registerResRoutes(api: Router): void {
  api.get('/models', route(async (_req, res) => {
    ok(res, modelsList())
  }))

  api.get('/providers', route(async (_req, res) => {
    ok(res, providersView())
  }))

  api.post('/providers/upsert', route(async (req, res) => {
    ok(res, providerUpsert(String(req.body?.name ?? ''), req.body?.provider ?? {}))
  }))

  api.post('/providers/delete', route(async (req, res) => {
    ok(res, providerDelete(String(req.body?.name ?? '')))
  }))

  api.post('/providers/models', route(async (req, res) => {
    ok(res, await providerProbe(req.body as ProviderProbeREQ))
  }))

  api.post('/providers/test', route(async (req, res) => {
    ok(res, await providerTest(req.body as ProviderProbeREQ))
  }))

  api.get('/login/providers', route(async (_req, res) => {
    ok(res, loginProviders())
  }))

  api.post('/login/start', route(async (req, res) => {
    ok(res, loginStart(String(req.body?.provider ?? '')))
  }))

  api.get('/skills', route(async (_req, res) => {
    ok(res, skillList())
  }))

  api.get('/skills/content', route(async (req, res) => {
    ok(res, skillContent(String(req.query.path ?? '')))
  }))

  api.post('/skills/save', route(async (req, res) => {
    ok(res, skillSave(String(req.body?.engine_kind ?? 'pi'), String(req.body?.file_path ?? ''), String(req.body?.text ?? '')))
  }))

  api.post('/skills/toggle', route(async (req, res) => {
    ok(res, skillToggle(String(req.body?.path ?? ''), req.body?.enabled === true))
  }))

  api.post('/skills/create', route(async (req, res) => {
    ok(
      res,
      skillCreate(String(req.body?.name ?? ''), String(req.body?.description ?? ''), String(req.body?.body ?? '')),
    )
  }))

  api.post('/skills/delete', route(async (req, res) => {
    ok(res, skillDelete(String(req.body?.path ?? '')))
  }))

  api.post('/skills/copy', route(async (req, res) => {
    ok(res, skillCopy(String(req.body?.path ?? '')))
  }))

  api.post('/skills/reset', route(async (req, res) => {
    ok(res, skillReset(String(req.body?.path ?? '')))
  }))

  api.get('/plugins', route(async (_req, res) => {
    ok(res, pluginList())
  }))

  api.post('/plugins/install', route(async (req, res) => {
    ok(res, await pluginInstall(String(req.body?.source ?? '')))
  }))

  api.post('/plugins/retry', route(async (req, res) => {
    ok(res, await pluginRetry(String(req.body?.plugin_name ?? '')))
  }))

  api.post('/plugins/retry-all', route(async (_req, res) => {
    pluginClearFailures()
    ok(res, await pluginInstallBuiltin())
  }))

  api.post('/plugins/uninstall', route(async (req, res) => {
    ok(res, await pluginUninstall(String(req.body?.source ?? '')))
  }))

  api.post('/plugins/toggle', route(async (req, res) => {
    ok(res, await pluginToggle(String(req.body?.plugin_name ?? ''), req.body?.enabled === true))
  }))

  api.post('/plugins/upgrade', route(async (req, res) => {
    ok(res, await pluginUpgrade(String(req.body?.plugin_name ?? '')))
  }))

  api.get('/mcp', route(async (req, res) => {
    ok(res, mcpList(req.query.live === '1'))
  }))

  api.post('/mcp/save', route(async (req, res) => {
    ok(res, mcpSave(req.body as McpSaveREQ))
  }))

  api.post('/mcp/toggle', route(async (req, res) => {
    ok(res, mcpToggle(String(req.body?.server_name ?? ''), req.body?.enabled === true))
  }))

  api.post('/mcp/remove', route(async (req, res) => {
    ok(res, mcpRemove(String(req.body?.server_name ?? '')))
  }))

  api.post('/mcp/copy', route(async (req, res) => {
    ok(res, mcpCopy(String(req.body?.server_name ?? ''), req.body?.enabled === true))
  }))
}
