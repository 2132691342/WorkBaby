// 时间展示：会话列表按「多近」描述，绝对时间只在超过一周后出现。
// 桌面用户看列表的直觉是「这是不是我刚才那段」，分钟/小时比 HH:mm 更快对上。

/** 相对时间：刚刚 / N 分钟前 / N 小时前 / 昨天 / N 天前 / M月D日 */
export function fmtRel(ms: number): string {
  if (!ms) return ''
  const now = Date.now()
  const diff = now - ms
  if (diff < 60_000) return '刚刚'
  if (diff < 3600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  const d = new Date(ms)
  const dayStart = new Date()
  dayStart.setHours(0, 0, 0, 0)
  if (ms >= dayStart.getTime()) {
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  }
  if (ms >= dayStart.getTime() - 86_400_000) return '昨天'
  if (diff < 7 * 86_400_000) return `${Math.floor(diff / 86_400_000)} 天前`
  return `${d.getMonth() + 1}月${d.getDate()}日`
}
