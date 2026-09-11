import { defineConfig } from 'vite'
import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// https://vite.dev/config/
export default defineConfig({
  base: './',
  plugins: [
    vue(),
    AutoImport({
      // 样式按需：ElementPlusResolver 默认 importStyle: 'css'，会为自动导入的组件
      // 注入 `element-plus/es/components/xx/style/css`，无需再引入全量 CSS。
      // 注意：命令式组件（ElMessage / ElMessageBox）与 v-loading 指令不经过模板扫描，
      // 其样式在 src/styles/element-services.scss 中显式补齐。
      resolvers: [ElementPlusResolver()],
      imports: ['vue', 'vue-router', 'pinia'],
      dts: 'src/auto-imports.d.ts'
    }),
    Components({
      resolvers: [ElementPlusResolver()],
      dts: 'src/components.d.ts'
    })
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  build: {
    chunkSizeWarningLimit: 1200,
    rollupOptions: {
      input: {
        main: fileURLToPath(new URL('./index.html', import.meta.url)),
        member: fileURLToPath(new URL('./member.html', import.meta.url))
      },
      output: {
        // 函数形式的分包：仅把"实际被打包（tree-shake 后）"的模块归入对应 chunk，
        // 避免数组形式强制引入整个依赖图导致按需引入失效。
        manualChunks(id) {
          if (id.includes('node_modules/element-plus') || id.includes('@element-plus/icons-vue')) {
            return 'element-plus'
          }
          if (id.includes('node_modules/echarts') || id.includes('node_modules/vue-echarts')) {
            return 'echarts'
          }
          if (id.includes('node_modules/vue') || id.includes('node_modules/vue-router') || id.includes('node_modules/pinia')) {
            return 'vue-vendor'
          }
        }
      }
    }
  },
  server: {
    port: 5173,
    host: true,
    proxy: {
      '/admin/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
})
