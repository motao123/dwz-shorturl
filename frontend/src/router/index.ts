import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import NProgress from 'nprogress'
import { useAuthStore } from '@/stores/auth'

NProgress.configure({ showSpinner: false, easing: 'ease', speed: 500 })

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/login/LoginView.vue'),
    meta: { title: '登录', public: true }
  },
  {
    path: '/',
    component: () => import('@/layouts/AdminLayout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/views/dashboard/DashboardView.vue'),
        meta: { title: '仪表盘', icon: 'Odometer', perm: 'stats.read' }
      },
      {
        path: 'short-urls',
        name: 'ShortUrls',
        component: () => import('@/views/short-urls/ShortUrlList.vue'),
        meta: { title: '短链管理', icon: 'Link', perm: 'short_urls.read' }
      },
      {
        path: 'users',
        name: 'Users',
        component: () => import('@/views/users/UserList.vue'),
        meta: { title: '用户管理', icon: 'User', perm: 'users.read' }
      },
      {
        path: 'roles',
        name: 'Roles',
        component: () => import('@/views/roles/RoleList.vue'),
        meta: { title: '角色管理', icon: 'Stamp', perm: 'roles.read' }
      },
      {
        path: 'stats',
        name: 'Stats',
        component: () => import('@/views/stats/StatsView.vue'),
        meta: { title: '统计分析', icon: 'TrendCharts', perm: 'stats.read' }
      },
      {
        path: 'configs',
        name: 'Configs',
        component: () => import('@/views/configs/ConfigView.vue'),
        meta: { title: '系统配置', icon: 'Setting', perm: 'configs.read' }
      },
      {
        path: 'audit-logs',
        name: 'AuditLogs',
        component: () => import('@/views/audit/AuditLogList.vue'),
        meta: { title: '审计日志', icon: 'Document', perm: 'audit.read' }
      },
      {
        path: 'api-keys',
        name: 'ApiKeys',
        component: () => import('@/views/api-keys/ApiKeyList.vue'),
        meta: { title: 'API 密钥', icon: 'Key', perm: 'api_keys.read' }
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/dashboard'
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach(async (to) => {
  NProgress.start()
  document.title = to.meta.title ? `${to.meta.title as string} · 短网址管理后台` : '短网址管理后台'

  const auth = useAuthStore()

  if (to.meta.public) {
    // 已登录用户访问登录页时直接回仪表盘
    if (auth.token && to.path === '/login') {
      return { path: '/dashboard' }
    }
    return true
  }

  if (!auth.token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  // 已持有 Token 但尚未拉取用户信息时补拉（含权限列表）
  if (!auth.userInfo) {
    try {
      await auth.fetchMe()
    } catch {
      auth.logoutLocal()
      return { path: '/login', query: { redirect: to.fullPath } }
    }
  }

  return true
})

router.afterEach(() => {
  NProgress.done()
})

export default router
