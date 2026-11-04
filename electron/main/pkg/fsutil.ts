//! 文件工具：原子写与目录保障，避免半截文件。
import fs from 'node:fs'
import path from 'node:path'

/** 确保目录存在（递归）。 */
export function ensureDir(dir: string): void {
  fs.mkdirSync(dir, { recursive: true })
}

/** 原子写文本：同目录临时文件 + rename，进程被杀也不会留半截。 */
export function writeAtomic(file: string, text: string): void {
  ensureDir(path.dirname(file))
  const tmp = `${file}.${process.pid}.tmp`
  fs.writeFileSync(tmp, text, 'utf8')
  fs.renameSync(tmp, file)
}

/** 读文本；不存在返回 null。 */
export function readText(file: string): string | null {
  try {
    return fs.readFileSync(file, 'utf8')
  } catch {
    return null
  }
}

/** 读 JSON；坏文件与不存在都返回 null。 */
export function readJson<T>(file: string): T | null {
  const text = readText(file)
  if (text == null) return null
  try {
    return JSON.parse(text) as T
  } catch {
    return null
  }
}

/** 写 JSON（两空格缩进 + 换行）。 */
export function writeJson(file: string, value: unknown): void {
  writeAtomic(file, `${JSON.stringify(value, null, 2)}\n`)
}

/** 毫秒时间戳；全仓时间一律 i64 毫秒。 */
export function nowMs(): number {
  return Date.now()
}
