/**
 * 编辑补丁合成：由 file_edit 的 old_string / new_string 在本地生成统一 diff。
 *
 * <p>补丁在审批「执行之前」就要给用户看——那时后端还没写过文件、也就没有 diff 可回传。
 * 这里只做公共前后缀裁剪 + 差异区列示（足够人读），不追求 LCS 最优：
 * 审批卡只需要「改哪一行、改成什么」。
 */

/** 合成 unified diff（`@@` 头 + `-`/`+` 行）；old/new 完全相同返回空串。 */
export function synthesizePatch(oldStr: string, newStr: string): string {
  if (oldStr === newStr) return ''
  const oldLines = splitLines(oldStr)
  const newLines = splitLines(newStr)

  let prefix = 0
  while (prefix < oldLines.length && prefix < newLines.length && oldLines[prefix] === newLines[prefix]) {
    prefix++
  }
  let suffix = 0
  while (
    suffix < oldLines.length - prefix &&
    suffix < newLines.length - prefix &&
    oldLines[oldLines.length - 1 - suffix] === newLines[newLines.length - 1 - suffix]
  ) {
    suffix++
  }

  const removed = oldLines.slice(prefix, oldLines.length - suffix)
  const added = newLines.slice(prefix, newLines.length - suffix)
  const out: string[] = [
    `@@ -${prefix + 1},${removed.length} +${prefix + 1},${added.length} @@`
  ]
  for (const l of removed) out.push(`- ${l}`)
  for (const l of added) out.push(`+ ${l}`)
  return out.join('\n')
}

/** 按行切分：保留空行（空行也是被编辑的内容），末尾换行不额外产生一行。 */
function splitLines(s: string): string[] {
  if (s === '') return []
  const lines = s.split('\n')
  if (lines.length > 1 && lines[lines.length - 1] === '') lines.pop()
  return lines
}
