// 上下文水位的取数规则：从消息回放与模型配置推出「已用 / 窗口」。
// 抽到 store 外只有一条理由——这套规则与「本轮运行态」无关，
// 重开会话、切模型、切分支三处都要用它。
import type { MessageVO, ModelVO } from '../types/api'

/** 窗口与输出上限的兜底：配置留空（0）会让水位环永远显示 0%。 */
export const FALLBACK_WINDOW = 300000
export const FALLBACK_MAX_TOKENS = 64000

/** 消息回放里最后一条带用量的助手消息的上下文占用；没有就 0。 */
export function lastContextOf(messages: MessageVO[]): number {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]
    if (m.message_role === 'assistant' && (m.message_usage?.usage_context || 0) > 0)
      return m.message_usage?.usage_context || 0
  }
  return 0
}

/** 按 provider / model id 找模型声明的窗口与输出上限（找不到用兜底）。 */
export function windowOfModels(
  list: ProviderLike[],
  provider: string,
  model: string,
): { window: number; max: number } {
  const pick = (m: ModelVO) => ({
    window: m.model_context_window > 0 ? m.model_context_window : FALLBACK_WINDOW,
    max: m.model_max_tokens > 0 ? m.model_max_tokens : FALLBACK_MAX_TOKENS,
  })
  const target = list.find((p) => p.provider_id === provider) ?? list[0]
  if (!target) return { window: FALLBACK_WINDOW, max: FALLBACK_MAX_TOKENS }
  const hit = target.provider_models.find((m) => m.model_id === model)
  if (hit) return pick(hit)
  for (const p of list) {
    const m = p.provider_models.find((x) => x.model_id === model)
    if (m) return pick(m)
  }
  return { window: FALLBACK_WINDOW, max: FALLBACK_MAX_TOKENS }
}

/** 只有窗口与模型清单这两个字段参与计算，收窄参数避免拉进整个 ProviderVO。 */
export interface ProviderLike {
  provider_id: string
  provider_models: ModelVO[]
}
