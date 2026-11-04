import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue(), UnoCSS()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      '@wails': fileURLToPath(new URL('./bindings', import.meta.url)),
    },
  },
  build: {
    target: 'es2022',
    chunkSizeWarningLimit: 1400,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['vue', 'vue-router', 'pinia', 'axios'],
          element: ['element-plus'],
          chart: ['echarts'],
        },
      },
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    // 显式绑 IPv4：Wails 的资产代理拨的是 127.0.0.1，
    // 默认 localhost 在本机只绑 ::1，代理侧于是 502。
    host: '127.0.0.1',
  },
})
