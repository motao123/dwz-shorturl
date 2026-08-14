import request, { type PageParams, type PageResult } from './request'

/** 公网注册用户状态：1=正常 0=禁用 */
export type MemberStatus = 0 | 1

export interface Member {
  id: number
  username: string
  email: string
  status: MemberStatus
  email_verified: number
  last_login_at: string | null
  last_login_ip: string | null
  created_at: string
}

export interface MemberQuery extends PageParams {
  keyword?: string
  status?: MemberStatus | ''
}

/** 公网注册用户分页列表 */
export function listMembers(params: MemberQuery): Promise<PageResult<Member>> {
  return request.get<PageResult<Member>>('/members', { params })
}

/** 启用/禁用 */
export function updateMemberStatus(id: number, status: MemberStatus): Promise<null> {
  return request.put<null>(`/members/${id}/status`, { status })
}

/** 重置密码 */
export function resetMemberPassword(id: number, password: string): Promise<null> {
  return request.put<null>(`/members/${id}/password`, { password })
}

/** 删除注册用户 */
export function removeMember(id: number): Promise<null> {
  return request.delete<null>(`/members/${id}`)
}