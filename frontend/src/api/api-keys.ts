import request, { type PageParams, type PageResult } from './request'

/** 密钥状态：1=有效 0=已吊销 */
export type ApiKeyStatus = 0 | 1

export interface ApiKey {
  id: number
  user_id: number
  name: string
  key_prefix: string
  permissions: string[] | null
  rate_limit: number
  last_used_at: string | null
  expires_at: string | null
  status: ApiKeyStatus
  created_at: string
}

export interface ApiKeyCreatePayload {
  name: string
  rate_limit?: number
  expires_days?: number | null
  permissions?: string[]
}

export interface ApiKeyCreateResult {
  id: number
  name: string
  /** 完整密钥明文，仅创建时返回一次 */
  api_key: string
  key_prefix: string
  expires_at: string | null
}

export interface ApiKeyStats {
  key_id: number
  total_requests: number
  today_requests: number
  recent: { label: string; count: number }[]
}

/** 密钥列表 */
export function listApiKeys(params?: PageParams): Promise<PageResult<ApiKey>> {
  return request.get<PageResult<ApiKey>>('/api-keys', { params })
}

/** 创建密钥（明文仅返回一次） */
export function createApiKey(data: ApiKeyCreatePayload): Promise<ApiKeyCreateResult> {
  return request.post<ApiKeyCreateResult>('/api-keys', data)
}

/** 吊销密钥 */
export function revokeApiKey(id: number): Promise<null> {
  return request.delete<null>(`/api-keys/${id}`)
}

/** 密钥调用统计 */
export function getApiKeyStats(id: number): Promise<ApiKeyStats> {
  return request.get<ApiKeyStats>(`/api-keys/${id}/stats`)
}
