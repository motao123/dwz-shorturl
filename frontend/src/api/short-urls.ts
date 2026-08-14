import request, { type PageParams, type PageResult } from './request'

/** 短链状态：1=启用 0=禁用 2=过期 */
export type ShortUrlStatus = 0 | 1 | 2

export interface ShortUrl {
  id: number
  uid: string
  long_url: string
  url_hash: string
  title: string | null
  category_id: number | null
  category_name?: string
  clicks: number
  status: ShortUrlStatus
  expire_at: string | null
  created_by: number | null
  creator_name?: string
  source: string
  ip: string | null
  domain_id?: number | null
  domain?: string
  has_password?: boolean
  short_url?: string
  created_at: string
  updated_at: string
}

export interface ShortUrlQuery extends PageParams {
  keyword?: string
  status?: ShortUrlStatus | ''
  category_id?: number | ''
  date_from?: string
  date_to?: string
  sort?: string
  order?: 'asc' | 'desc'
  /** 1=只看已删除（回收站） */
  include_deleted?: 1 | 0
}

export interface ShortUrlPayload {
  long_url: string
  uid?: string
  title?: string
  category_id?: number | null
  expire_days?: number | null
  expire_at?: string | null
  status?: ShortUrlStatus
  domain_id?: number | null
  /** 访问密码；编辑时省略=不修改，空串=清除 */
  password?: string
}

/** 分页列表 */
export function listShortUrls(params: ShortUrlQuery): Promise<PageResult<ShortUrl>> {
  return request.get<PageResult<ShortUrl>>('/short-urls', { params })
}

/** 详情 */
export function getShortUrl(id: number): Promise<ShortUrl> {
  return request.get<ShortUrl>(`/short-urls/${id}`)
}

/** 创建短链 */
export function createShortUrl(data: ShortUrlPayload): Promise<ShortUrl> {
  return request.post<ShortUrl>('/short-urls', data)
}

/** 编辑短链 */
export function updateShortUrl(id: number, data: Partial<ShortUrlPayload>): Promise<ShortUrl> {
  return request.put<ShortUrl>(`/short-urls/${id}`, data)
}

/** 删除（软删除） */
export function removeShortUrl(id: number): Promise<null> {
  return request.delete<null>(`/short-urls/${id}`)
}

/** 批量删除 */
export function batchRemoveShortUrls(ids: number[]): Promise<{ deleted: number }> {
  return request.post<{ deleted: number }>('/short-urls/batch-delete', { ids })
}

/** 恢复已删除短链（回收站） */
export function restoreShortUrl(id: number): Promise<ShortUrl> {
  return request.post<ShortUrl>(`/short-urls/${id}/restore`)
}

/** 批量更新（状态/有效期） */
export function batchUpdateShortUrls(ids: number[], data: { status?: number; expire_days?: number }): Promise<{ updated: number }> {
  return request.post<{ updated: number }>('/short-urls/batch-update', { ids, ...data })
}

export interface LinkStat {
  uid: string
  total: number
  trend: { label: string; clicks: number }[]
  referrers: { label: string; clicks: number }[]
  referrer_types?: { label: string; clicks: number }[]
  devices?: { label: string; clicks: number }[]
  browsers?: { label: string; clicks: number }[]
  countries?: { label: string; clicks: number }[]
}

/** 单链统计数据 */
export function getShortUrlStats(id: number): Promise<LinkStat> {
  return request.get<LinkStat>(`/stats/link/${id}`)
}

export interface LinkCheckResult {
  id: number
  url: string
  ok: boolean
  status: number
  error?: string
}

/** 链接健康检查（HEAD 目标 URL） */
export function checkShortUrl(id: number): Promise<LinkCheckResult> {
  return request.get<LinkCheckResult>(`/short-urls/${id}/check`)
}

export interface BatchCreateResult {
  results: ShortUrl[]
  errors: string[]
  total: number
}

/** 批量创建 */
export function batchCreateShortUrls(urls: string[], domainId?: number | null): Promise<BatchCreateResult> {
  return request.post<BatchCreateResult>('/short-urls/batch-create', {
    urls,
    domain_id: domainId || undefined
  })
}

/** 导出 CSV（二进制流） */
export function exportShortUrlsCsv(params: Omit<ShortUrlQuery, 'page' | 'per_page'>): Promise<Blob> {
  return request.get<Blob>('/short-urls/export', { params, responseType: 'blob' })
}

export interface ImportResult {
  results: ShortUrl[]
  errors: string[]
  total: number
}

export interface ImportShortUrlPayload {
  format: 'csv' | 'json'
  content: string
  domain_id?: number | null
}

/** CSV/JSON 导入 */
export function importShortUrls(data: ImportShortUrlPayload): Promise<ImportResult> {
  return request.post<ImportResult>('/short-urls/import', {
    format: data.format,
    content: data.content,
    domain_id: data.domain_id || undefined
  })
}
