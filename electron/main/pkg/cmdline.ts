//! 命令行拆分：把「可执行 + 参数」的整行拆成 argv 数组，引号内不拆。

/** 拆一行命令：支持双引号 / 单引号 / 反引号包裹的参数。 */
export function splitCommandLine(text: string): string[] {
  const out: string[] = []
  const re = /"([^"]*)"|'([^']*)'|`([^`]*)`|(\S+)/g
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    out.push(m[1] ?? m[2] ?? m[3] ?? m[4])
  }
  return out
}
