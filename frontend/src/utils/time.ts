// 时间展示：会话列表按「多近」描述，绝对时间只在超过一周后出现。
// 桌面用户看列表的直觉是「这是不是我刚才那段」，分钟/小时比 HH:mm 更快对上。
// 文案走 i18n：时间读数是界面文案，不是数据。
import { i18n } from '../i18n'

/** 相对时间：刚刚 / N 分钟前 / HH:mm / 昨天 / N 天前 / M月D日。 */
export function fmtRel(ms: number): string {
  if (!ms) return ''
  const t = i18n.global.t
  const diff = Date.now() - ms
  if (diff < 60_000) return t('time.justNow')
  if (diff < 3600_000) return t('time.minutesAgo', { n: Math.floor(diff / 60_000) })
  const d = new Date(ms)
  const dayStart = new Date()
  dayStart.setHours(0, 0, 0, 0)
  if (ms >= dayStart.getTime()) {
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  }
  if (ms >= dayStart.getTime() - 86_400_000) return t('time.yesterday')
  if (diff < 7 * 86_400_000) return t('time.daysAgo', { n: Math.floor(diff / 86_400_000) })
  return t('time.monthDay', { month: d.getMonth() + 1, day: d.getDate() })
}

/** 绝对时间：YYYY-MM-DD HH:mm，用于提交与历史这类要精确到分钟的读数。 */
export function fmtAbs(ms: number): string {
  if (!ms) return ''
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
