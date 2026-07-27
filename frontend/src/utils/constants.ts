/** 短链对外域名（用于拼接完整短链） */
export const SHORT_DOMAIN = 'https://dwz.cn'

/** 拼接完整短链 */
export function buildShortUrl(uid: string): string {
  return `${SHORT_DOMAIN}/${uid}`
}

/** 短链状态展示映射 */
export const SHORT_URL_STATUS: Record<number, { label: string; type: 'success' | 'info' | 'warning' }> = {
  1: { label: '启用', type: 'success' },
  0: { label: '禁用', type: 'info' },
  2: { label: '已过期', type: 'warning' }
}

/** 用户状态展示映射 */
export const USER_STATUS: Record<number, { label: string; type: 'success' | 'info' }> = {
  1: { label: '正常', type: 'success' },
  0: { label: '禁用', type: 'info' }
}

/** API 密钥状态展示映射 */
export const API_KEY_STATUS: Record<number, { label: string; type: 'success' | 'danger' }> = {
  1: { label: '有效', type: 'success' },
  0: { label: '已吊销', type: 'danger' }
}

/** 短链分组（对应 url_categories 表） */
export const URL_CATEGORIES: { id: number; name: string }[] = [
  { id: 1, name: '营销推广' },
  { id: 2, name: '社交媒体' },
  { id: 3, name: '线下二维码' },
  { id: 4, name: '邮件投放' },
  { id: 5, name: '其他' }
]

export function categoryName(id: number | null | undefined): string {
  return URL_CATEGORIES.find((c) => c.id === id)?.name ?? '—'
}

/** 来源展示 */
export const SOURCE_LABELS: Record<string, string> = {
  web: '网页',
  api: 'API',
  batch: '批量',
  admin: '后台'
}
