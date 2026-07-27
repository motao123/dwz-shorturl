import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as authApi from '@/api/auth'
import type { LoginParams, MeInfo } from '@/api/auth'
import { tokenStorage } from '@/api/request'

const USER_KEY = 'dwz_user_info'

function readCachedUser(): MeInfo | null {
  try {
    const raw = localStorage.getItem(USER_KEY)
    return raw ? (JSON.parse(raw) as MeInfo) : null
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref(tokenStorage.token)
  const refreshToken = ref(tokenStorage.refreshToken)
  const userInfo = ref<MeInfo | null>(readCachedUser())

  const isLoggedIn = computed(() => Boolean(token.value))
  const displayName = computed(
    () => userInfo.value?.display_name || userInfo.value?.username || '未登录'
  )
  const roles = computed<string[]>(() => userInfo.value?.roles ?? [])
  const permissions = computed<string[]>(() => userInfo.value?.permissions ?? [])
  const isSuperAdmin = computed(() => roles.value.includes('super_admin'))

  function setTokens(access: string, refresh?: string) {
    token.value = access
    tokenStorage.token = access
    if (refresh) {
      refreshToken.value = refresh
      tokenStorage.refreshToken = refresh
    }
  }

  /** 登录 */
  async function login(params: LoginParams): Promise<void> {
    const res = await authApi.login(params)
    setTokens(res.access_token, res.refresh_token)
    // 登录响应里的 user 不含权限明细，随后拉取 /me 获取完整权限
    await fetchMe()
  }

  /** 拉取当前用户信息 + 权限列表 */
  async function fetchMe(): Promise<MeInfo> {
    const me = await authApi.getMe()
    userInfo.value = me
    localStorage.setItem(USER_KEY, JSON.stringify(me))
    return me
  }

  /** 刷新 Token */
  async function refreshAccessToken(): Promise<void> {
    if (!refreshToken.value) throw new Error('无可用的 Refresh Token')
    const res = await authApi.refreshToken(refreshToken.value)
    setTokens(res.access_token, res.refresh_token)
  }

  /** 仅清理本地状态（用于 401 被动登出） */
  function logoutLocal() {
    token.value = ''
    refreshToken.value = ''
    userInfo.value = null
    tokenStorage.clear()
    localStorage.removeItem(USER_KEY)
  }

  /** 主动登出：通知后端拉黑 Token，再清理本地 */
  async function logout(): Promise<void> {
    if (token.value) {
      try {
        await authApi.logout()
      } catch {
        // 后端不可达时也允许本地登出
      }
    }
    logoutLocal()
  }

  return {
    token,
    refreshToken,
    userInfo,
    isLoggedIn,
    displayName,
    roles,
    permissions,
    isSuperAdmin,
    login,
    fetchMe,
    refreshAccessToken,
    logout,
    logoutLocal,
    setTokens
  }
})
