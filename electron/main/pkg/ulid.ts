//! ID 生成：裸 ULID 加业务前缀；时间序即排序序。
import { ulid } from 'ulid'

/** 允许的业务前缀白名单：新增实体类型先扩这里。 */
export type Prefix = 'RUN' | 'ASK' | 'EVT' | 'JOB'

const PREFIXES: readonly Prefix[] = ['RUN', 'ASK', 'EVT', 'JOB']

/** 生成一个带前缀的 ULID，形如 RUN_01JQ8K2M3N4P5Q6R7S8T9V0WXY。 */
export function newId(prefix: Prefix): string {
  return `${prefix}_${ulid()}`
}

/** 校验一个 ID 的形状与前缀合法性。 */
export function isValidId(value: string): boolean {
  const idx = value.indexOf('_')
  if (idx <= 0) return false
  const prefix = value.slice(0, idx)
  if (!PREFIXES.includes(prefix as Prefix)) return false
  const body = value.slice(idx + 1)
  return /^[0-9A-HJKMNP-TV-Z]{26}$/.test(body)
}
