import { createApp } from 'vue'
import { createPinia } from 'pinia'

import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import 'nprogress/nprogress.css'
import '@/styles/index.scss'

import App from './App.vue'
import router from './router'
import { initTheme } from '@/stores/theme'

initTheme()

const app = createApp(App)

// 按需引入：模板组件由 unplugin-vue-components 自动导入，命令式组件
// （ElMessage 等）已改为 element-plus/es 深路径导入（见 vite.config.ts），
// 全量 CSS 保留以兜底样式。中文 locale 由 App.vue 的 ElConfigProvider 注入。
app.use(createPinia())
app.use(router)

app.mount('#app')
