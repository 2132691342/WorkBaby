//! markdown 前置元数据：SKILL.md 等文件的 YAML 头部字段读取。

/** 从 `--- ... ---` 头部抠单行字段；无头给空。 */
export function parseFrontmatter(text: string): Record<string, string> {
  const m = /^---\r?\n([\s\S]*?)\r?\n---/.exec(text)
  const out: Record<string, string> = {}
  if (!m) return out
  for (const line of m[1].split(/\r?\n/)) {
    const kv = /^([A-Za-z_-]+):\s*(.*)$/.exec(line.trim())
    if (kv) out[kv[1]] = kv[2].trim()
  }
  return out
}
