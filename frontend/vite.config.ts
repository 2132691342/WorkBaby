import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'

/**
 * Vite 配置 · WorkBaby Go 版（Wails v2 嵌入式前端）。
 *
 * <p>关键点：
 * <ul>
 *   <li>实际源码位于 src/src/*（整段复用 Java 版 webapp）；此处仅做 alias 指向</li>
 *   <li>Wails 嵌入式：build.outDir 必须是 dist（与 //go:embed all:frontend/dist 一致）</li>
 *   <li>WebView2 CSP：生产构建收紧（无 unsafe-eval）；dev 仍由 wails dev 注入</li>
 *   <li>无 /api 代理、无 /ws 代理——所有调用走 Wails 绑定（@/wailsjs/go/main/App）</li>
 * </ul>
 */
export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
    AutoImport({
      imports: ['vue', 'vue-router', 'pinia'],
      dts: 'src/src/auto-import.d.ts',
      eslintrc: { enabled: false }
    }),
    Components({
      dts: 'src/src/components.d.ts',
      dirs: ['src/src/components'],
      extensions: ['vue'],
      deep: true
    })
  ],
  resolve: {
    // 数组顺序匹配：长的 prefix 放最前面，避免 '@/wailsjs' 被 '@' 抢先解析为 src/src/wailsjs
    alias: [
      { find: '@/wailsjs', replacement: fileURLToPath(new URL('./src/wailsjs', import.meta.url)) },
      { find: '@', replacement: fileURLToPath(new URL('./src/src', import.meta.url)) }
    ]
  },
  server: {
    port: 5173,
    strictPort: false,
    host: '127.0.0.1'
    // wails dev 启动时会接管此 dev server；前端单独 `npm run dev` 仅用来调样式，调不到 Go 绑定
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: true,
    target: 'es2022',
    // 手动分块（PI Phase 5 优化）：重型依赖按需加载，首屏 bundle 仅含 Vue/Pinia/router/axios。
    //  - mermaid / cytoscape / katex / echarts / markdown-it 都已通过路由级 dynamic import 引入，
    //    manualChunks 进一步把它们从主 chunk 拆出 → 单页首屏 < 600KB（gzip 前）。
    //  - 路由级 dynamic import 见 ChatStreamDecoder.ts / markdown 渲染器 / dashboard 视图。
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          if (id.includes('mermaid')) return 'vendor-mermaid'
          if (id.includes('cytoscape')) return 'vendor-cytoscape'
          if (id.includes('katex')) return 'vendor-katex'
          if (id.includes('echarts') || id.includes('zrender')) return 'vendor-echarts'
          if (id.includes('markdown-it') || id.includes('highlight.js') || id.includes('dompurify')) {
            return 'vendor-markdown'
          }
          // 运行时核心（始终在主 bundle）
          if (id.includes('vue') || id.includes('pinia') || id.includes('vue-router')
            || id.includes('axios') || id.includes('@vue') || id.includes('element-plus')
            || id.includes('@element-plus')) {
            return 'vendor'
          }
          return undefined
        }
      }
    }
  }
})
