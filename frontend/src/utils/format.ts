// 读数的显示格式：token 数与耗时。
// 抽成独立模块是因为两处要用同一份口径——轮尾读数与活动条上的输出计数，
// 两处各自实现一遍必然出现「同一轮在两处显示 1.2k 与 1234」。

/** token 数按量级压缩：界面上要能一眼比较，不是要精确到个位。 */
export function fmtCount(n: number): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 100_000 ? 0 : 1)}k`
  return String(n)
}

/** 耗时：毫秒在 1 秒以下没有意义（用户看不出差别），秒以上带一位小数。 */
export function fmtMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`
}
