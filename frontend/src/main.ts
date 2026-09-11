import { createApp } from 'vue'
import { createPinia } from 'pinia'

// Element Plus 样式：底座 + 暗色变量（src/styles/element/index.scss），
// 组件 CSS 由 ElementPlusResolver 按需注入；命令式组件/指令样式见 element-services.scss。
import '@/styles/element/index.scss'
import '@/styles/element-services.scss'
import 'nprogress/nprogress.css'
import '@/styles/index.scss'

import App from './App.vue'
import router from './router'
import { initTheme } from '@/stores/theme'

initTheme()

const app = createApp(App)

// 按需引入：模板组件与其 CSS 均由 unplugin-vue-components 自动注入；
// 命令式组件（ElMessage / ElMessageBox / v-loading）样式在
// src/styles/element-services.scss 中显式补齐（模板扫描不到）。
// 中文 locale 由 App.vue 的 ElConfigProvider 注入。
app.use(createPinia())
app.use(router)

app.mount('#app')
