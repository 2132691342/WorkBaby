//! 内置帮助文档：assets/docs 的 markdown 清单与内容。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import { wrapError } from '../pkg/apperror'
import { CODE_FILE_READ, CODE_NOT_FOUND } from '../domain/errors'
import type { HelpDocVO, HelpDocRESP } from '../domain/types_data'

const MAX_CONTENT = 400_000

/** 文档清单：文件名去掉 .md 作 doc_name，首个一级标题作标题。 */
export function docsList(): HelpDocVO[] {
  const dir = path.join(paths().wb_assets_dir, 'docs')
  let names: string[] = []
  try {
    names = fs.readdirSync(dir).filter((n) => n.endsWith('.md'))
  } catch {
    return []
  }
  return names.map((n) => {
    const content = safeRead(path.join(dir, n))
    const heading = /^#\s+(.+)$/m.exec(content)
    return { doc_name: n.replace(/\.md$/, ''), doc_title: heading?.[1]?.trim() ?? n.replace(/\.md$/, '') }
  })
}

/** 读一篇文档；越界（白名单外）或缺失都报业务码。 */
export function docsContent(name: string): HelpDocRESP {
  if (!/^[\w.-]+$/.test(name)) throw wrapError(CODE_NOT_FOUND, '文档不存在', new Error(`非法文档名 ${name}`))
  const file = path.join(paths().wb_assets_dir, 'docs', `${name}.md`)
  if (!fs.existsSync(file)) throw wrapError(CODE_NOT_FOUND, '文档不存在', new Error(file))
  const text = safeRead(file)
  if (text.length > MAX_CONTENT) {
    return { doc_content: text.slice(0, MAX_CONTENT) }
  }
  return { doc_content: text }
}

function safeRead(file: string): string {
  try {
    return fs.readFileSync(file, 'utf8')
  } catch (err) {
    throw wrapError(CODE_FILE_READ, `文档读取失败：${path.basename(file)}`, err)
  }
}
