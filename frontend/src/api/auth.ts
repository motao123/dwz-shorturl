import request from './request'

export interface LoginParams {
  username: string
  password: string
  captcha?: string
  captcha_id?: string
}

export interface LoginUser {
  id: number
  username: string
  email?: string
  display_name?: string
  avatar_url?: string
  roles: string[]
}

export interface LoginResult {
  access_token: string
  refresh_token: string
  expires_in: number
  user: LoginUser
}

export interface MeInfo {
  id: number
  username: string
  email: string
  display_name: string
  avatar_url: string
  status: number
  last_login_at: string | null
  last_login_ip: string | null
  roles: string[]
  permissions: string[]
}

/** 用户名密码登录 */
export function login(data: LoginParams): Promise<LoginResult> {
  return request.post<LoginResult>('/auth/login', data)
}

/** 登出（Token 加入黑名单） */
export function logout(): Promise<null> {
  return request.post<null>('/auth/logout')
}

/** 刷新 Access Token */
export function refreshToken(refresh_token: string): Promise<LoginResult> {
  return request.post<LoginResult>('/auth/refresh', { refresh_token })
}

/** 获取当前用户信息 + 权限列表 */
export function getMe(): Promise<MeInfo> {
  return request.get<MeInfo>('/auth/me')
}
