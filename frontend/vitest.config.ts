import { defineConfig } from 'vitest/config'
import { fileURLToPath, URL } from 'node:url'

// 单元测试只覆盖逻辑层（composables / utils），不加载 .vue 组件，
// 因此无需 @vitejs/plugin-vue，保持测试启动开销最小。
export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  test: {
    environment: 'node',
    include: ['src/**/__tests__/**/*.spec.ts'],
    globals: false
  }
})
