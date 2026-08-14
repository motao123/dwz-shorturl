import { createApp } from 'vue'
import { createPinia } from 'pinia'

import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import '@/styles/index.scss'

import App from './member/App.vue'
import router from './member/router'
import { initTheme } from '@/stores/theme'

initTheme()

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')