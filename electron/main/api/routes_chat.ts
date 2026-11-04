//! 聊天路由：chat 全家桶 + 消息隐藏 + 子代理空实现。
import type { Router } from 'express'
import { ok, route } from '../pkg/resp'
import type { ChatAttachmentVO, UIResponseVO } from '../domain/types_chat'
import {
  chatState,
  chatSend,
  chatStop,
  chatSteer,
  chatFollowUp,
  chatClearQueue,
  chatRemoveQueued,
  chatCompact,
  chatAnswer,
  chatNew,
  chatClone,
  chatSetModel,
  chatSetThinking,
  chatSetAutoCompaction,
} from '../service/chat/chat_service'
import { hideTurn, unhideTurn, sessionExport } from '../service/session_service'

function asAttachments(raw: unknown): ChatAttachmentVO[] {
  return Array.isArray(raw) ? (raw as ChatAttachmentVO[]) : []
}

export function registerChatRoutes(api: Router): void {
  api.get('/chat/state', route(async (req, res) => {
    ok(res, await chatState(req.query.session ? String(req.query.session) : undefined))
  }))

  api.post('/chat/send', route(async (req, res) => {
    ok(
      res,
      await chatSend(
        String(req.body?.engine ?? 'pi'),
        String(req.body?.session ?? ''),
        String(req.body?.message ?? ''),
        asAttachments(req.body?.attachments),
      ),
    )
  }))

  api.post('/chat/stop', route(async (req, res) => {
    ok(res, await chatStop())
  }))

  api.post('/chat/steer', route(async (req, res) => {
    ok(
      res,
      await chatSteer(String(req.body?.message ?? ''), asAttachments(req.body?.attachments)),
    )
  }))

  api.post('/chat/followup', route(async (req, res) => {
    ok(
      res,
      await chatFollowUp(String(req.body?.message ?? ''), asAttachments(req.body?.attachments)),
    )
  }))

  api.post('/chat/clear-queue', route(async (_req, res) => {
    ok(res, await chatClearQueue())
  }))

  api.post('/chat/remove-queued', route(async (_req, res) => {
    ok(res, await chatRemoveQueued())
  }))

  api.post('/chat/compact', route(async (req, res) => {
    ok(res, await chatCompact(String(req.body?.instructions ?? '')))
  }))

  api.post('/chat/answer', route(async (req, res) => {
    const request = (req.body?.request ?? {}) as UIResponseVO
    ok(res, await chatAnswer(String(req.body?.session ?? ''), request))
  }))

  api.post('/chat/new', route(async (_req, res) => {
    ok(res, await chatNew())
  }))

  api.post('/chat/clone', route(async (_req, res) => {
    ok(res, await chatClone())
  }))

  api.post('/chat/export-html', route(async (req, res) => {
    ok(res, sessionExport(String(req.body?.session ?? '')))
  }))

  api.post('/chat/model', route(async (req, res) => {
    ok(res, await chatSetModel(String(req.body?.provider ?? ''), String(req.body?.model ?? '')))
  }))

  api.post('/chat/thinking', route(async (req, res) => {
    ok(res, await chatSetThinking(String(req.body?.level ?? 'medium')))
  }))

  api.post('/chat/autocompact', route(async (req, res) => {
    ok(res, await chatSetAutoCompaction(req.body?.enabled === true))
  }))

  api.post('/messages/hide', route(async (req, res) => {
    hideTurn(String(req.body?.session ?? ''), String(req.body?.hid ?? ''))
    ok(res, { ok: true })
  }))

  api.post('/messages/unhide', route(async (req, res) => {
    unhideTurn(String(req.body?.session ?? ''), String(req.body?.hid ?? ''))
    ok(res, { ok: true })
  }))

  // pi 无子代理：给空清单让面板安静地空着。
  api.get('/agents', route(async (_req, res) => {
    ok(res, { agents: [] })
  }))

  api.get('/agents/messages', route(async (_req, res) => {
    ok(res, { entries: [], messages: [], next: 0, reset: true })
  }))

  api.post('/agents/steer', route(async (_req, res) => {
    ok(res, { ok: true })
  }))

  api.post('/agents/cancel', route(async (_req, res) => {
    ok(res, { cancelled: true })
  }))

  api.post('/agents/subscribe', route(async (_req, res) => {
    ok(res, { ok: true })
  }))
}
