import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

/**
 * Vite 配置：Wails v2 嵌入式前端。
 *
 * <p>build.outDir 必须是 dist，与 Go 侧 //go:embed all:frontend/dist 对应。
 * <p>开发调试界面可用 `npm run dev`；调通后端请用 `wails dev`（会注入 app:ready 端口）。
 */
export default defineConfig({
  // EP 按需引入：全量注册会把 21 个用到的组件连同全部样式打进首屏，
  // 解析器按模板里真实出现的标签按需注册。
  plugins: [
    vue(),
    UnoCSS(),
    Components({
      dts: false,
      resolvers: [ElementPlusResolver()],
    }),
  ],
  resolve: {
    alias: [
      { find: '@/wailsjs', replacement: fileURLToPath(new URL('./wailsjs', import.meta.url)) },
      { find: '@', replacement: fileURLToPath(new URL('./src', import.meta.url)) },
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
          if (id.includes('echarts') || id.includes('zrender')) {
            return 'vendor-echarts'
          }
          // EP 不归堆：按需导入的组件归进单个 chunk 会把内部依赖一起拉回首屏，
          // 让未使用组件的 tree-shaking 与各视图的懒加载同时失效。
          if (id.includes('element-plus') || id.includes('@element-plus')) return undefined
          if (id.includes('@vueuse')) return 'vendor-ep'
          if (id.includes('codemirror')) {
            return 'vendor-cm'
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
