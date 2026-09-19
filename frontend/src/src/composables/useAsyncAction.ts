import { ref, type Ref } from 'vue'

/**
 * 异步操作包装：把任意 async 函数包成带 {@code pending} 状态 + 错误捕获的版本，UI 可直接绑定按钮 disabled / loading。
 * 支持并发（计数 inflight）、错误抛出 + 写 error、类型透传。Vue / Pinia 双栈可用。
 */
export interface AsyncAction<F extends (...args: any[]) => Promise<any>> {
  /** 包装后的函数（签名 / 返回类型与原函数一致）。 */
  run: F
  /** 是否有未完成的调用（ref；store 内可解构出 reactive 字段）。 */
  pending: Ref<boolean>
  /** 最近一次错误；null 表示未出错。 */
  error: Ref<Error | null>
  /** 清除 error 状态。 */
  reset: () => void
}

export function useAsyncAction<F extends (...args: any[]) => Promise<any>>(
  fn: F
): AsyncAction<F> {
  const pending = ref(false)
  const error = ref<Error | null>(null)
  // 用计数而非单一标志位，支持"上一次请求还未回，下一次已发"的并发场景
  let inflight = 0

  const run = (async (...args: Parameters<F>) => {
    inflight += 1
    pending.value = true
    error.value = null
    try {
      return await fn(...args)
    } catch (e) {
      error.value = e instanceof Error ? e : new Error(String(e))
      throw error.value
    } finally {
      inflight -= 1
      if (inflight <= 0) {
        inflight = 0
        pending.value = false
      }
    }
  }) as F

  const reset = (): void => {
    error.value = null
  }

  return { run, pending, error, reset }
}