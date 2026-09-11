import { createApp } from 'vue'
import { createPinia } from 'pinia'

import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import '@/styles/index.scss'

import App from './member/App.vue'
import router from './member/router'
import { initTheme } from '@/stores/theme'

// 会员端是独立入口（member.html），初始的深/浅色由这里决定；用户切换后
// 由当前页面的 themeStore 自行持久化，无论停留在哪个入口都能生效。
initTheme()

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')