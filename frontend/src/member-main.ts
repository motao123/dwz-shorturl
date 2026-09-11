import { createApp } from 'vue'
import { createPinia } from 'pinia'

// Element Plus 样式：与 admin 入口保持同一策略（底座 + 暗色变量 + 按需组件样式）。
import '@/styles/element/index.scss'
import '@/styles/element-services.scss'
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