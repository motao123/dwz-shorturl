import axios, {
  AxiosError,
  type AxiosInstance,
  type AxiosRequestConfig,
  type InternalAxiosRequestConfig
} from 'axios'

/** 后端统一响应包裹 */
export interface ApiEnvelope<T = unknown> {
  code: number
  msg: string
  data: T
}

/** 分页响应结构 */
export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  per_page: number
}

/** 分页查询公共参数 */
export interface PageParams {
  page?: number
  per_page?: number
}

const TOKEN_KEY = 'dwz_access_token'
const REFRESH_KEY = 'dwz_refresh_token'

export const tokenStorage = {
  get token(): string {
    return localStorage.getItem(TOKEN_KEY) ?? ''
  },
  set token(v: string) {
    v ? localStorage.setItem(TOKEN_KEY, v) : localStorage.removeItem(TOKEN_KEY)
  },
  get refreshToken(): string {
    return localStorage.getItem(REFRESH_KEY) ?? ''
  },
  set refreshToken(v: string) {
    v ? localStorage.setItem(REFRESH_KEY, v) : localStorage.removeItem(REFRESH_KEY)
  },
  clear() {
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(REFRESH_KEY)
  }
}

const service: AxiosInstance = axios.create({
  baseURL: '/admin/api',
  timeout: 15000
})

/* ---------------- 请求拦截器 ---------------- */

service.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = tokenStorage.token
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

/* ---------------- 响应拦截器 ---------------- */

// Token 刷新单飞控制
let refreshing = false
let pendingQueue: Array<(token: string) => void> = []

function flushQueue(token: string) {
  pendingQueue.forEach((cb) => cb(token))
  pendingQueue = []
}

function hardLogout() {
  tokenStorage.clear()
  localStorage.removeItem('dwz_user_info')
  const redirect = encodeURIComponent(window.location.pathname + window.location.search)
  window.location.href = `/login?redirect=${redirect}`
}

async function doRefreshToken(): Promise<string> {
  const refreshToken = tokenStorage.refreshToken
  if (!refreshToken) throw new Error('missing refresh token')
  // 使用裸 axios，避免走拦截器造成死循环
  const { data } = await axios.post<ApiEnvelope<{ access_token: string; refresh_token?: string }>>(
    '/admin/api/auth/refresh',
    { refresh_token: refreshToken }
  )
  const payload = data?.data
  if (data?.code !== 0 || !payload?.access_token) {
    throw new Error(data?.msg || 'refresh failed')
  }
  tokenStorage.token = payload.access_token
  if (payload.refresh_token) tokenStorage.refreshToken = payload.refresh_token
  return payload.access_token
}

service.interceptors.response.use(
  (response) => {
    // 二进制流直接返回 Blob 本体
    if (response.config.responseType === 'blob') {
      return response.data as never
    }

    const body = response.data as ApiEnvelope | Record<string, unknown>

    // 统一包裹格式：{ code, msg, data }
    if (body && typeof body === 'object' && 'code' in body) {
      const envelope = body as ApiEnvelope
      if (envelope.code === 0) {
        return envelope.data as never
      }
      const err = new Error(envelope.msg || '请求失败') as Error & { code?: number }
      err.code = envelope.code
      return Promise.reject(err)
    }

    // 兼容直接返回数据的后端
    return body as never
  },
  async (error: AxiosError<ApiEnvelope>) => {
    const original = error.config as (InternalAxiosRequestConfig & { _retried?: boolean }) | undefined
    const status = error.response?.status
    const msg = error.response?.data?.msg

    // 401：尝试用 Refresh Token 静默续期并重放请求
    if (
      status === 401 &&
      original &&
      !original._retried &&
      !original.url?.includes('/auth/login') &&
      !original.url?.includes('/auth/refresh') &&
      tokenStorage.refreshToken
    ) {
      original._retried = true

      if (refreshing) {
        // 已在刷新中：排队等待新 Token
        return new Promise((resolve) => {
          pendingQueue.push((token: string) => {
            original.headers.Authorization = `Bearer ${token}`
            resolve(service(original))
          })
        })
      }

      refreshing = true
      try {
        const token = await doRefreshToken()
        flushQueue(token)
        original.headers.Authorization = `Bearer ${token}`
        return service(original)
      } catch {
        flushQueue('')
        pendingQueue = []
        hardLogout()
        return Promise.reject(new Error('登录已过期，请重新登录'))
      } finally {
        refreshing = false
      }
    }

    if (status === 401) {
      hardLogout()
    }

    const fallback = status ? `请求失败（${status}）` : '网络异常，请检查后端服务'
    const finalMsg = msg || fallback
    const err = new Error(finalMsg) as Error & { code?: number; status?: number }
    err.status = status
    err.code = error.response?.data?.code
    return Promise.reject(err)
  }
)

/* ---------------- 便捷方法（直接返回 data） ---------------- */

const request = {
  get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return service.get<never, T>(url, config)
  },
  post<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return service.post<never, T>(url, data, config)
  },
  put<T>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return service.put<never, T>(url, data, config)
  },
  delete<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return service.delete<never, T>(url, config)
  }
}

export default request
