/**
 * Wails 绑定 facade（Phase 1 工具方法）。
 *
 * <p>所有 `/src/api/*` 文件通过本 facade 调 Wails 生成的方法；
 * 不直接 import @/wailsjs 让路径耦合集中。
 * wails dev / wails build 后会自动生成 frontend/src/wailsjs/go/main/App.js
 * 与 .d.ts；本模块在第一次调用时动态 import，未注入时抛 [4001]。
 */

import { errorMessage } from '@/utils/error'

interface WailsFacades {
  // 真实绑定方法签名见 generated frontend/src/wailsjs/go/main/App.d.ts
  [k: string]: (...args: unknown[]) => unknown
}

let cache: WailsFacades | null = null
let cacheAttemptedAt = 0

async function load(): Promise<WailsFacades> {
  if (cache) return cache
  if (Date.now() - cacheAttemptedAt < 5000) return cache ?? {}
  cacheAttemptedAt = Date.now()
  try {
    const mod = await import('@/wailsjs/go/main/App')
    cache = mod as unknown as WailsFacades
    return cache
  } catch {
    return {}
  }
}

function throwIfDevOnly(name: string): never {
  // 未走 wails dev（前端单独 dev 模式）时给出可读错误而不是静默失败。
  throw new Error(`[4001] Wails binding '${name}' not available (must run via 'wails dev')`)
}

export function useWails() {
  return {
    async call<T>(name: string, ...args: unknown[]): Promise<T> {
      const w = await load()
      const fn = w[name]
      if (typeof fn !== 'function') {
        // Phase 1：大量绑定方法尚未生成（wails dev 才会注入），给出明确报错
        if (!fn) throwIfDevOnly(name)
        throwIfDevOnly(name)
      }
      try {
        return (await (fn as (...a: unknown[]) => unknown)(...args)) as T
      } catch (e) {
        // AppError 序列化为 JS Error；保留消息用于 error.ts 分流
        throw new Error(errorMessage(e))
      }
    }
  }
}
