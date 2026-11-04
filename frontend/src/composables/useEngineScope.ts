// 管理页的引擎作用域。
//
// 本壳只有一个引擎，作用域这个概念已经没有意义：要么全看，要么没得看。
// 保留这个模块是为了让调用方不必为「只有一个」写特例，
// 同时把「将来可能不止一个」的扩展点留在这一处。
import { ref } from 'vue'

/** 作用域：唯一引擎，或「全部」（当前与唯一引擎等价）。 */
export type EngineScope = 'pi' | 'all'

/**
 * 一个页面的作用域状态。
 *
 * page 是持久化用的键后缀（models / skills / plugins / mcp），互不干扰。
 * 只有一个引擎时它的值恒为 'pi'，set 是空操作。
 */
export function useEngineScope(page: string) {
  const key = `wb.scope.${page}`
  const scope = ref<EngineScope>('pi')
  void key
  function set(next: EngineScope) {
    scope.value = next === 'all' ? 'all' : 'pi'
  }
  return { scope, set }
}
