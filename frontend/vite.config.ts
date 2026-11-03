import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue(), UnoCSS()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
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
    // 显式绑 IPv4：Electron 开发态加载的就是这个地址，
    // 默认 localhost 在本机可能只绑 ::1，加载侧会连不上。
    host: '127.0.0.1',
  },
})
