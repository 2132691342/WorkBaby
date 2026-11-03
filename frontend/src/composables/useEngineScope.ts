// 管理页的引擎作用域：omp / pi / 全部。每页一份而不复用对话的「当前引擎」——
// 「我在跟谁说话」与「我在管谁的东西」是两个问题，绑在一起会顺手切丢当前会话。
// 落在 sessionStorage：页面间反复进出要用，但不该跨重启保留。
import { ref } from 'vue'
import type { EngineKind } from '@/types/api'

/** 作用域：某个引擎，或两个都要。 */
export type EngineScope = EngineKind | 'all'

const KEY_PREFIX = 'omp.scope.'

/**
 * 一个页面的作用域状态。
 *
 * page 是持久化用的键后缀（models / skills / plugins / mcp），互不干扰：
 * 看技能时选「全部」不该让模型页也变成全部。
 */
export function useEngineScope(page: string, fallback: EngineScope = 'all') {
  const key = KEY_PREFIX + page
  const stored = sessionStorage.getItem(key)
  const scope = ref<EngineScope>(normalize(stored) ?? fallback)

  function set(next: EngineScope) {
    scope.value = next
    sessionStorage.setItem(key, next)
  }

  return { scope, set }
}

// normalize 把存下来的字符串收窄成合法作用域；非法值当没存过。
function normalize(raw: string | null): EngineScope | null {
  if (raw === 'omp' || raw === 'pi' || raw === 'all') return raw
  return null
}
