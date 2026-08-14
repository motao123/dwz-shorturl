// Member console API layer.
// Auth: PHP member.php (session cookie) issues a member JWT.
// Data: Go /member/api/* endpoints (X-Member-Token header).

let memberToken = ''
let memberCsrf = ''

export function setMemberToken(t: string) {
  memberToken = t
}

export function getMemberToken(): string {
  return memberToken
}

/** 获取 CSRF token；没有则先拉一次会话（member.php 默认 action 会刷新 csrf） */
async function ensureCsrf(): Promise<string> {
  if (memberCsrf) return memberCsrf
  await fetchSession()
  return memberCsrf
}

async function phpHtml(action: string, params: Record<string, string> = {}): Promise<any> {
  const body = new URLSearchParams(params)
  body.set('action', action)
  if (action === 'login' || action === 'register' || action === 'logout') {
    body.set('csrf', await ensureCsrf())
  }
  const res = await fetch('/member.php', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8' },
    body: body.toString(),
    credentials: 'same-origin'
  })
  const data = await res.json()
  if (data.result !== 1) throw new Error(data.msg || '操作失败')
  return data
}

/** 获取当前登录态 + 会员 token（member.php 默认 action） */
export async function fetchSession(): Promise<{ member: any; token: string }> {
  const body = new URLSearchParams()
  const res = await fetch('/member.php', {
    method: 'POST',
    credentials: 'same-origin',
    body: body.toString()
  })
  const data = await res.json()
  const m = data.data?.member || null
  const token = data.data?.token || ''
  memberCsrf = data.data?.csrf || memberCsrf
  setMemberToken(token)
  return { member: m, token }
}

export async function login(username: string, password: string): Promise<any> {
  const data = await phpHtml('login', { username, password })
  setMemberToken(data.data?.token || '')
  return data.data?.member
}

/** 请求重置密码（发送邮件） */
export async function requestPasswordReset(email: string): Promise<void> {
  await go('/member/api/auth/forgot-password', {
    method: 'POST',
    body: JSON.stringify({ email })
  })
}

/** 用 token 重置密码 */
export async function resetPassword(token: string, password: string): Promise<void> {
  await go('/member/api/auth/reset-password', {
    method: 'POST',
    body: JSON.stringify({ token, password })
  })
}

/** 发送邮箱验证邮件 */
export async function sendVerification(email: string): Promise<void> {
  await go('/member/api/auth/send-verification', {
    method: 'POST',
    body: JSON.stringify({ email })
  })
}

/** 用 token 验证邮箱 */
export async function verifyEmail(token: string): Promise<void> {
  await go('/member/api/auth/verify-email', {
    method: 'POST',
    body: JSON.stringify({ token })
  })
}

export async function register(username: string, email: string, password: string): Promise<any> {
  const data = await phpHtml('register', { username, email, password })
  setMemberToken(data.data?.token || '')
  return data.data?.member
}

export async function logout(): Promise<void> {
  try {
    await phpHtml('logout', {})
  } catch {
    /* ignore */
  }
  setMemberToken('')
}

async function go(path: string, opts: RequestInit = {}): Promise<any> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(opts.headers as Record<string, string>)
  }
  if (memberToken) headers['X-Member-Token'] = memberToken
  const res = await fetch(path, { ...opts, headers, credentials: 'same-origin' })
  const data = await res.json()
  if (data.code !== 0) throw new Error(data.msg || '请求失败')
  return data.data
}

export interface MemberLink {
  id: number
  uid: string
  long_url: string
  title?: string | null
  clicks: number
  expire_at: string | null
  created_at: string
  short_url?: string
}

export function buildShortUrl(uid: string): string {
  return `${location.origin}/${uid}`
}

export interface MemberSummary {
  total_links: number
  total_clicks: number
  month_new: number
}

export async function getSummary(): Promise<MemberSummary> {
  return go('/member/api/summary')
}

/** 导出我的短链为 CSV（直接触发浏览器下载） */
export async function exportLinksCsv(): Promise<void> {
  const res = await fetch('/member/api/links/export', {
    headers: memberToken ? { 'X-Member-Token': memberToken } : {},
    credentials: 'same-origin'
  })
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.msg || '导出失败')
  }
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'my_links.csv'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

export async function getMyLinks(page = 1, perPage = 20, keyword = '', status = ''): Promise<{ list: MemberLink[]; total: number }> {
  const q = new URLSearchParams({ page: String(page), per_page: String(perPage) })
  if (keyword) q.set('keyword', keyword)
  if (status) q.set('status', status)
  return go(`/member/api/links?${q.toString()}`)
}

/** 获取目标网页标题 */
export async function fetchTitle(url: string): Promise<string> {
  const r = await go('/member/api/links/fetch-title', {
    method: 'POST',
    body: JSON.stringify({ url })
  })
  return r.title as string
}

export async function createLink(url: string, custom = '', expireDays = 0, title = ''): Promise<MemberLink> {
  return go('/member/api/links', {
    method: 'POST',
    body: JSON.stringify({ url, title, custom, expire_days: expireDays })
  })
}

export interface MemberBatchResult {
  url: string
  uid: string
  short_url: string
  error?: string
}

export async function importLinks(content: string): Promise<MemberBatchResult[]> {
  return go('/member/api/links/import', {
    method: 'POST',
    body: JSON.stringify({ content })
  })
}

export async function batchCreateLinks(urls: string[]): Promise<MemberBatchResult[]> {
  return go('/member/api/links/batch', {
    method: 'POST',
    body: JSON.stringify({ urls })
  })
}

export async function deleteLink(id: number): Promise<void> {
  return go(`/member/api/links/${id}`, { method: 'DELETE' })
}

export interface LinkStat {
  uid: string
  total: number
  trend: { date: string; clicks: number }[]
  referrers: { referrer: string; count: number }[]
  referrer_types: { device: string; count: number }[]
  devices: { device: string; count: number }[]
  browsers: { device: string; count: number }[]
  countries: { device: string; count: number }[]
}

export async function getLinkStats(uid: string): Promise<LinkStat> {
  return go(`/member/api/links/${uid}/stats`)
}

/** 一键续期全部已过期/即将过期的链接 */
export async function renewExpiring(expireDays: number): Promise<{ renewed: number }> {
  return go('/member/api/links/renew-expiring', {
    method: 'POST',
    body: JSON.stringify({ expire_days: expireDays })
  })
}

export async function updateLinkExpiry(id: number, expireDays: number): Promise<{ expire_at: string | null }> {
  return go(`/member/api/links/${id}/expiry`, {
    method: 'PUT',
    body: JSON.stringify({ expire_days: expireDays })
  })
}

export async function updateLink(id: number, data: { long_url?: string; title?: string; expire_days?: number }): Promise<MemberLink> {
  return go(`/member/api/links/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data)
  })
}