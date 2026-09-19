import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

/**
 * Vitest 配置 · WorkBaby 前端。
 *
 * <p>组件测试（@vue/test-utils mount）需要 DOM 环境 → 全局 jsdom；
 * 纯函数测试（decoder/batcher）在 jsdom 下同样可跑。alias 与 vite.config.ts 保持一致。
 */
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src/src', import.meta.url))
    }
  },
  test: {
    environment: 'jsdom'
  }
})
