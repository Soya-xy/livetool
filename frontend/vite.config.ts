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
  build: { outDir: 'dist', emptyOutDir: true },
})
