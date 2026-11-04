//! pi RPC 客户端：把 pi --mode rpc 子进程包成「请求-应答 + 事件流」。
import { spawn, type ChildProcess } from 'node:child_process'
import { toolEnv } from '../../pkg/env'
import { paths } from '../../context'
import { logError, logWarn } from '../../pkg/log'
import { CODE_RPC_CMD, CODE_RPC_DOWN, CODE_RPC_TIMEOUT } from '../../domain/errors'
import { appError } from '../../pkg/apperror'
import { piEntry, piInstalled, nodeExe } from '../engine/runtime'

/** pi 事件记录（session 事件与 UI 请求共用）。 */
export type PiRecord = Record<string, unknown>

interface Pending {
  resolve: (data: unknown) => void
  reject: (err: Error) => void
  timer: ReturnType<typeof setTimeout>
  command: string
}

const REQUEST_TIMEOUT = 10 * 60_000

export interface RpcHandlers {
  on_event: (record: PiRecord) => void
  on_ui_request: (record: PiRecord) => void
  on_exit: (code: number | null) => void
}

/**
 * 一个工作区一个 RPC 子进程；请求按 id 关联，事件原样上抛。
 * stdout 严格按 LF 切帧（不用 readline，避免 U+2028/2029 被误切）。
 */
export class PiRpc {
  private child: ChildProcess | null = null
  private pending = new Map<string, Pending>()
  private rest = ''
  private seq = 0
  private exited = false
  readonly started_at = Date.now()
  call_count = 0
  last_error: string | undefined

  constructor(
    private readonly cwd: string,
    private readonly handlers: RpcHandlers,
  ) {}

  /** 进程是否还活着。 */
  isAlive(): boolean {
    return this.child !== null && !this.exited && this.child.exitCode === null
  }

  /** 子进程 pid；没起给 0。 */
  get pid(): number {
    return this.child?.pid ?? 0
  }

  /** 拉起子进程并开始泵 stdout。 */
  start(): void {
    const p = paths()
    if (!piInstalled()) {
      throw appError(CODE_RPC_DOWN, 'pi 尚未安装，请先在应用页安装')
    }
    // PI_PACKAGE_DIR 会改写 pi 自己找 dist/主题的根，指错等于让引擎起不来，绝不设。
    const env = toolEnv({
      PI_CODING_AGENT_DIR: p.wb_pi_agent_dir,
      // 壳自己管升级，引擎不必再探一次远端版本（离线机器上会卡启动）。
      PI_SKIP_VERSION_CHECK: '1',
      NODE_NO_WARNINGS: '1',
    })
    this.child = spawn(nodeExe(), [piEntry(), '--mode', 'rpc'], {
      cwd: this.cwd,
      env,
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'],
    })
    this.child.stdout?.setEncoding('utf8')
    this.child.stdout?.on('data', (chunk: string) => this.consume(chunk))
    const stderr = this.child.stderr
    stderr?.setEncoding('utf8')
    stderr?.on('data', (chunk: string) => {
      const line = chunk.trim()
      if (line) logWarn(`pi: ${line.slice(0, 300)}`)
    })
    this.child.on('error', (err) => {
      this.last_error = err.message
      logError(`pi 子进程错误：${err.message}`)
      this.failAll(CODE_RPC_DOWN, 'pi 子进程启动失败')
    })
    this.child.on('close', (code) => {
      this.exited = true
      this.child = null
      this.failAll(CODE_RPC_DOWN, 'pi 子进程已退出')
      this.handlers.on_exit(code)
    })
  }

  /** 发一条命令并等它的 response。 */
  request(command: Record<string, unknown>, timeout_ms = REQUEST_TIMEOUT): Promise<unknown> {
    if (!this.isAlive()) throw appError(CODE_RPC_DOWN, 'pi 未在运行')
    this.seq += 1
    const id = `wb-${this.seq}`
    const wire = { id, ...command }
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(appError(CODE_RPC_TIMEOUT, `pi 命令超时：${String(command.type)}`))
      }, timeout_ms)
      this.pending.set(id, { resolve, reject, timer, command: String(command.type) })
      this.call_count += 1
      this.child?.stdin?.write(`${JSON.stringify(wire)}\n`)
    })
  }

  /** 发一条无需应答的记录（extension_ui_response 用）。 */
  send(record: Record<string, unknown>): void {
    if (!this.isAlive()) throw appError(CODE_RPC_DOWN, 'pi 未在运行')
    this.child?.stdin?.write(`${JSON.stringify(record)}\n`)
  }

  /** 收一帧 stdout：缓冲到 LF 为止。 */
  private consume(chunk: string): void {
    this.rest += chunk
    let idx = this.rest.indexOf('\n')
    while (idx >= 0) {
      const line = this.rest.slice(0, idx).replace(/\r$/, '')
      this.rest = this.rest.slice(idx + 1)
      if (line.trim()) this.dispatch(line)
      idx = this.rest.indexOf('\n')
    }
  }

  private dispatch(line: string): void {
    let record: PiRecord
    try {
      record = JSON.parse(line) as PiRecord
    } catch {
      logWarn(`pi 非 JSON 输出：${line.slice(0, 200)}`)
      return
    }
    const type = String(record.type ?? '')
    if (type === 'response') {
      const id = String(record.id ?? '')
      const pending = this.pending.get(id)
      if (!pending) return
      this.pending.delete(id)
      clearTimeout(pending.timer)
      if (record.success === false) {
        pending.reject(appError(CODE_RPC_CMD, String(record.error ?? 'pi 命令失败')))
      } else {
        pending.resolve(record.data)
      }
      return
    }
    if (type === 'extension_ui_request') {
      this.handlers.on_ui_request(record)
      return
    }
    this.handlers.on_event(record)
  }

  /** 关停：关 stdin 触发 pi 优雅退出，超时强杀。 */
  stop(): void {
    const child = this.child
    if (!child) return
    this.failAll(CODE_RPC_DOWN, 'pi 正在关停')
    try {
      child.stdin?.end()
    } catch {
      /* 已关就不管 */
    }
    const killer = setTimeout(() => {
      try {
        child.kill('SIGKILL')
      } catch {
        /* 已死就算了 */
      }
    }, 5000)
    killer.unref()
  }

  private failAll(code: string, message: string): void {
    for (const [, pending] of this.pending) {
      clearTimeout(pending.timer)
      pending.reject(appError(code, message))
    }
    this.pending.clear()
  }
}
