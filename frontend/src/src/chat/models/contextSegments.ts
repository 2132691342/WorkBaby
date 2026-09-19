/**
 * 上下文段颜色映射（与 ContextRing / ContextUsagePopover 共用）。
 *
 * <p>原本两处各维护一份 SEGMENT_COLOR 映射，新增段类型会因漏改一处而出现
 * 「环上是蓝色、列表上是灰色」的不一致——集中此模块后类型与色值同源。
 *
 * <p>只放纯函数（无 Vue 依赖），便于 service 层或测试模块复用。
 */
export const SEGMENT_COLOR: Readonly<Record<string, string>> = Object.freeze({
  system: 'var(--wb-primary)',
  memory: 'var(--wb-lavender)',
  tools: 'var(--wb-sky)',
  history: 'var(--wb-mint)'
})

/** 取某段的颜色；未知段回落主色（与历史一致，避免漏改一处时环/列表出现暗色）。 */
export function colorForSegment(key: string): string {
  return SEGMENT_COLOR[key] ?? 'var(--wb-primary)'
}
