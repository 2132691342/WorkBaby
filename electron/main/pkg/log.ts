//! 日志：按天滚动落盘 + 控制台镜像；业务层只写 info/warn/error。
import fs from 'node:fs'
import path from 'node:path'
import { ensureDir } from './fsutil'

let log_dir = ''
let log_date = ''
let log_stream: fs.WriteStream | null = null

/** 初始化日志目录；不初始化就只打控制台。 */
export function initLog(dir: string): void {
  log_dir = dir
  ensureDir(dir)
}

/** 最近日志原文（设置页日志面板用）：读今天的文件尾部。 */
export function recentLines(limit = 400): string[] {
  const file = currentFile()
  if (!file) return []
  try {
    const text = fs.readFileSync(file, 'utf8')
    return text.split('\n').slice(-limit).filter(Boolean)
  } catch {
    return []
  }
}

function stamp(): string {
  return new Date().toISOString().replace('T', ' ').replace('Z', '')
}

export function logInfo(msg: string): void {
  write('INFO', msg)
}

export function logWarn(msg: string): void {
  write('WARN', msg)
}

export function logError(msg: string): void {
  write('ERROR', msg)
}

function currentFile(): string | null {
  return log_dir ? path.join(log_dir, `wb-${log_date}.log`) : null
}

function write(level: string, msg: string): void {
  const line = `${stamp()} [${level}] ${msg}`
  if (level === 'ERROR') console.error(line)
  else console.log(line)
  if (!log_dir) return
  const today = new Date().toISOString().slice(0, 10)
  if (today !== log_date || !log_stream) {
    log_date = today
    log_stream?.end()
    ensureDir(log_dir)
    log_stream = fs.createWriteStream(path.join(log_dir, `wb-${today}.log`), { flags: 'a' })
  }
  log_stream.write(`${line}\n`)
}

/** 进程退出前收尾。 */
export function closeLog(): void {
  log_stream?.end()
  log_stream = null
}
