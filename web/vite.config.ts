import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8090',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: '../server/dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // 独立 chunk：hls.js 约 410KB，单独打包按需加载；element-plus 图标独立
        manualChunks(id) {
          if (id.includes('hls.js')) return 'vendor-hls'
          if (id.includes('@element-plus/icons-vue')) return 'vendor-ep-icons'
          if (id.includes('element-plus')) return 'vendor-ep'
          if (id.includes('node_modules')) return 'vendor'
        }
      }
    }
  }
})
