import request, { type PageParams, type PageResult } from './request'

/** 违规审核：0=待审核 1=已归档 */
export type Reviewed = 0 | 1

export interface ViolationReview {
  id: number
  url: string
  reason: string
  ip: string
  source: string
  reviewed: Reviewed
  reviewed_at: string | null
  note: string
  created_at: string
}

export interface ViolationQuery extends PageParams {
  reviewed?: Reviewed | ''
  keyword?: string
}

/** 违规审核分页列表 */
export function listViolations(params: ViolationQuery): Promise<PageResult<ViolationReview>> {
  return request.get<PageResult<ViolationReview>>('/violations', { params })
}

/** 标记已审 */
export function markViolationReviewed(id: number, note: string): Promise<null> {
  return request.put<null>(`/violations/${id}/review`, { note })
}

/** 删除记录 */
export function removeViolation(id: number): Promise<null> {
  return request.delete<null>(`/violations/${id}`)
}