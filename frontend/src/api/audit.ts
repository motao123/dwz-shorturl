import request, { type PageParams, type PageResult } from './request'

export interface AuditLog {
  id: number
  user_id: number | null
  username?: string
  action: string
  resource: string | null
  resource_id: string | null
  detail: Record<string, unknown> | null
  ip: string
  user_agent: string | null
  created_at: string
}

export interface AuditLogQuery extends PageParams {
  user_id?: number | ''
  action?: string
  date_from?: string
  date_to?: string
}

/** 审计日志分页列表 */
export function listAuditLogs(params: AuditLogQuery): Promise<PageResult<AuditLog>> {
  return request.get<PageResult<AuditLog>>('/audit-logs', { params })
}

/** 审计日志详情 */
export function getAuditLog(id: number): Promise<AuditLog> {
  return request.get<AuditLog>(`/audit-logs/${id}`)
}
