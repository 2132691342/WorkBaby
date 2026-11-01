// 读数格式：仪表盘与上下文水位共用，避免各处各写一套 K/M 规则。
// 单位走 i18n：中文跟单位、英文跟缩写是两种习惯，不能靠前端写死。
import { i18n } from '../i18n'

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
  if (v < 1000) return i18n.global.t('num.millis', { n: v })
  return i18n.global.t('num.seconds', { n: trim(v / 1000) })
}

/** 字节数按量级缩写：1536 → 1.5 KB */
export function fmtBytes(n: number): string {
  const v = Math.max(0, Math.round(n || 0))
  if (v < 1024) return `${v} B`
  if (v < 1024 * 1024) return `${trim(v / 1024)} KB`
  return `${trim(v / 1024 / 1024)} MB`
}

/** 花费（美元）：小额保留三位，大额两位——0.003 显示成 0.00 等于没说 */
export function fmtMoney(usd: number): string {
  const v = Math.max(0, usd || 0)
  if (v === 0) return '$0'
  if (v < 0.01) return `$${v.toFixed(4)}`
  if (v < 1) return `$${v.toFixed(3)}`
  return `$${v.toFixed(2)}`
}
