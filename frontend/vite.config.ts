import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import wails from '@wailsio/runtime/plugins/vite'

const root = fileURLToPath(new URL('.', import.meta.url))

export default defineConfig({
  root,
  publicDir: fileURLToPath(new URL('../resources', import.meta.url)),
  plugins: [
    vue(),
    Components({ dirs: [], dts: 'src/components.d.ts', resolvers: [ElementPlusResolver()] }),
    wails('./bindings'),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      '@shared': fileURLToPath(new URL('./src/shared', import.meta.url)),
    },
  },
  server: { host: '127.0.0.1', port: Number(process.env.WAILS_VITE_PORT || 9245), strictPort: true },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // three 是锁链特效专用的懒加载 chunk（约 540 kB / gzip 134 kB），只在组件窗打开时加载；
    // 这里放宽阈值，避免每次构建都为这个已知的 3D 依赖报 chunk 体积警告。
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        // Three.js 只被组件窗（锁链特效）用到，单独成 chunk：页面 chunk 保持精简，也便于缓存。
        manualChunks: (id) => (id.includes('node_modules/three') ? 'three' : undefined),
      },
    },
  },
})
