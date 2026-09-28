import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

/**
 * Vite 配置：Wails v2 嵌入式前端。
 *
 * <p>build.outDir 必须是 dist，与 Go 侧 //go:embed all:frontend/dist 对应。
 * <p>开发调试界面可用 `npm run dev`；调通后端请用 `wails dev`（会注入 app:ready 端口）。
 */
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: [
      { find: '@/wailsjs', replacement: fileURLToPath(new URL('./src/wailsjs', import.meta.url)) },
      { find: '@', replacement: fileURLToPath(new URL('./src/src', import.meta.url)) },
    ],
  },
  server: {
    port: 5173,
    strictPort: false,
    host: '127.0.0.1',
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    target: 'es2022',
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          if (id.includes('highlight.js') || id.includes('dompurify') || id.includes('marked')) {
            return 'vendor-markdown'
          }
          if (id.includes('vue') || id.includes('pinia') || id.includes('vue-router') || id.includes('axios')) {
            return 'vendor'
          }
          return undefined
        },
      },
    },
  },
})
