import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import NProgress from 'nprogress'
import { useAuthStore } from '@/stores/auth'
import { hasPermission } from '@/utils/permission'

NProgress.configure({ showSpinner: false, easing: 'ease', speed: 500 })

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/login/LoginView.vue'),
    meta: { title: '登录', public: true }
  },
  {
    path: '/403',
    name: 'Forbidden',
    component: () => import('@/views/error/ForbiddenView.vue'),
    meta: { title: '无权限', public: true }
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
        path: 'domains',
        name: 'Domains',
        component: () => import('@/views/domains/DomainList.vue'),
        meta: { title: '域名管理', icon: 'Connection', perm: 'domains.read' }
      },
      {
        path: 'users',
        name: 'Users',
        component: () => import('@/views/users/UserList.vue'),
        meta: { title: '用户管理', icon: 'User', perm: 'users.read' }
      },
      {
        path: 'members',
        name: 'Members',
        component: () => import('@/views/members/MemberList.vue'),
        meta: { title: '注册用户', icon: 'UserFilled', perm: 'users.read' }
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
        path: 'monitor',
        name: 'Monitor',
        component: () => import('@/views/monitor/MonitorView.vue'),
        meta: { title: '系统监控', icon: 'Monitor', perm: 'stats.read' }
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
        path: 'violations',
        name: 'Violations',
        component: () => import('@/views/violations/ViolationReviewList.vue'),
        meta: { title: '违规审核', icon: 'WarningFilled', perm: 'audit.read' }
      },
      {
        path: 'api-keys',
        name: 'ApiKeys',
        component: () => import('@/views/api-keys/ApiKeyList.vue'),
        meta: { title: 'API 密钥', icon: 'Key', perm: 'api_keys.read' }
      },
      {
        path: 'api-docs',
        name: 'ApiDocs',
        component: () => import('@/views/api-docs/ApiDocsView.vue'),
        meta: { title: 'API 文档', icon: 'Document', perm: 'api_keys.read' }
      },
      {
        path: 'webhooks',
        name: 'Webhooks',
        component: () => import('@/views/webhooks/WebhookList.vue'),
        meta: { title: 'Webhook', icon: 'BellFilled', perm: 'api_keys.read' }
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/dashboard'
  }
]

const router = createRouter({
  history: createWebHistory('/admin/'),
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

  // P1: 路由级权限校验，防止低权限用户直接改 URL 进入无权限页面
  if (to.meta.perm && !hasPermission(to.meta.perm as string)) {
    return { path: '/403' }
  }

  return true
})

router.afterEach(() => {
  NProgress.done()
})

export default router
