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
    target: 'es2022'
  }
})
