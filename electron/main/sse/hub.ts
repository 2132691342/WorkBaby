//! SSE 广播中心：seq 单调递增、断线按 since 补帧、心跳保活。
import type { Response } from 'express'
import { newId } from '../pkg/ulid'

interface StreamFrame {
  seq: number
  event: string
  data: Record<string, unknown>
}

interface SseClient {
  res: Response
  last_seq: number
}

const clients = new Set<SseClient>()
const buffer: StreamFrame[] = []
const BUFFER_MAX = 1000
let seq = 0
let heartbeat: ReturnType<typeof setInterval> | null = null

/** 接入一个 SSE 客户端；since 之前缺的帧尽力补，补不上交给前端 reconcile。 */
export function attach(res: Response, since: number): void {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  })
  const client: SseClient = { res, last_seq: since }
  clients.add(client)
  for (const frame of buffer) {
    if (frame.seq > since) write(client, frame)
  }
  res.write(`event: message\ndata: ${JSON.stringify({ seq, event: 'stream:open', data: {} })}\n\n`)
  if (!heartbeat) {
    heartbeat = setInterval(() => {
      for (const c of [...clients]) {
        try {
          c.res.write(': ping\n\n')
        } catch {
          clients.delete(c)
        }
      }
    }, 25_000)
    heartbeat.unref()
  }
  res.on('close', () => clients.delete(client))
}

/** 广播一帧；seq 由这里统一发号。 */
export function broadcast(event: string, data: Record<string, unknown> = {}): void {
  seq += 1
  const frame: StreamFrame = { seq, event, data }
  buffer.push(frame)
  if (buffer.length > BUFFER_MAX) buffer.shift()
  for (const client of [...clients]) {
    if (frame.seq > client.last_seq) write(client, frame)
  }
}

/** 当前 seq：日志与调试用。 */
export function currentSeq(): number {
  return seq
}

function write(client: SseClient, frame: StreamFrame): void {
  client.last_seq = frame.seq
  try {
    client.res.write(`event: message\ndata: ${JSON.stringify(frame)}\n\n`)
  } catch {
    clients.delete(client)
  }
}

/** 跑一个带 run_id 的广播：聊天帧统一从这里出。 */
export function broadcastRun(event: string, run_id: string, data: Record<string, unknown> = {}): void {
  broadcast(event, { run_id, engine_kind: 'pi', ...data })
}

/** 生成一个运行 ID。 */
export function newRunId(): string {
  return newId('RUN')
}
