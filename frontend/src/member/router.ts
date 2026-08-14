import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { fetchSession } from './api'

const routes: RouteRecordRaw[] = [
  { path: '/login', name: 'Login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
  { path: '/reset', name: 'ResetPassword', component: () => import('./views/ResetPasswordView.vue'), meta: { public: true } },
  { path: '/verify', name: 'VerifyEmail', component: () => import('./views/VerifyEmailView.vue'), meta: { public: true } },
  { path: '/', name: 'Dashboard', component: () => import('./views/DashboardView.vue') },
  { path: '/:pathMatch(.*)*', redirect: '/' }
]

const router = createRouter({
  history: createWebHistory('/member/'),
  routes
})

router.beforeEach(async (to) => {
  try {
    const { member } = await fetchSession()
    if (to.path === '/login' && member) return { path: '/' }
    if (!to.meta.public && !member) return { path: '/login' }
  } catch {
    if (!to.meta.public) return { path: '/login' }
  }
  return true
})

export default router