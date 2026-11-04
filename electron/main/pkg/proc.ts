//! 子进程出口：argv 数组直传、不进 shell、windowsHide（即 CREATE_NO_WINDOW）、超时杀。
import { spawn } from 'node:child_process'

export interface ProcResult {
  proc_code: number
  proc_stdout: string
  proc_stderr: string
}

export interface ProcOptions {
  /** 工作目录 */
  cwd?: string
  /** 额外环境变量（PATH 已在调用方排好序） */
  env?: Record<string, string>
  /** 超时毫秒；超时杀进程。0 表示不限 */
  timeout_ms?: number
  /** 输入到 stdin 的内容 */
  stdin?: string
  /** 逐行回调（stdout）；给了它就不再攒整段输出 */
  on_line?: (line: string) => void
}

/** 跑一个进程到退出。失败（找不到可执行文件）抛错，非零码由调用方按业务判断。 */
export function runProc(exe: string, args: string[], options: ProcOptions = {}): Promise<ProcResult> {
  return new Promise((resolve, reject) => {
    const child = spawn(exe, args, {
      cwd: options.cwd,
      env: options.env ? { ...process.env, ...options.env } : process.env,
      // windowsHide 即 CREATE_NO_WINDOW：控制台窗口不会闪。
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'],
    })
    child.once('error', reject)

    let stdout = ''
    let stderr = ''
    let rest = ''
    child.stdout.setEncoding('utf8')
    child.stdout.on('data', (chunk: string) => {
      if (options.on_line) {
        rest += chunk
        let idx = rest.indexOf('\n')
        while (idx >= 0) {
          options.on_line(stripCr(rest.slice(0, idx)))
          rest = rest.slice(idx + 1)
          idx = rest.indexOf('\n')
        }
      } else {
        stdout += chunk
      }
    })
    child.stderr.setEncoding('utf8')
    child.stderr.on('data', (chunk: string) => {
      stderr += chunk
    })

    let killed = false
    const timer =
      options.timeout_ms && options.timeout_ms > 0
        ? setTimeout(() => {
            killed = true
            child.kill()
          }, options.timeout_ms)
        : null

    child.on('close', (code) => {
      if (timer) clearTimeout(timer)
      if (options.on_line && rest) options.on_line(stripCr(rest))
      if (killed) {
        reject(new Error(`进程超时被杀：${exe}`))
        return
      }
      resolve({ proc_code: code ?? -1, proc_stdout: stdout, proc_stderr: stderr })
    })

    if (options.stdin != null) {
      child.stdin.end(options.stdin)
    } else {
      child.stdin.end()
    }
  })
}

function stripCr(line: string): string {
  return line.endsWith('\r') ? line.slice(0, -1) : line
}
