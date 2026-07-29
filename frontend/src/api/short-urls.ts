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
