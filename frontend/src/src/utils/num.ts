// 读数格式：仪表盘与上下文水位共用，避免各处各写一套 K/M 规则。

/** token 数按量级缩写：128000 → 128K */
export function fmtCount(n: number): string {
  const v = Math.max(0, Math.round(n || 0))
  if (v < 1000) return String(v)
  if (v < 1_000_000) return `${trim(v / 1000)}K`
  return `${trim(v / 1_000_000)}M`
}

function trim(n: number): string {
  return (n < 10 ? n.toFixed(1) : String(Math.round(n))).replace(/\.0$/, '')
}

/** 精确整数（千分位）：给需要照着填进设置的数字用，缩写会读不出准数 */
export function fmtInt(n: number): string {
  return String(Math.max(0, Math.round(n || 0))).replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

/** 耗时用秒，够读就行：830 → 0.8 秒 */
export function fmtMs(ms: number): string {
  const v = Math.max(0, Math.round(ms || 0))
  if (v < 1000) return `${v} 毫秒`
  return `${trim(v / 1000)} 秒`
}
