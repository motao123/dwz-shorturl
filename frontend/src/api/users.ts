import request, { type PageParams, type PageResult } from './request'

/** 用户状态：1=正常 0=禁用 */
export type UserStatus = 0 | 1

export interface AdminUser {
  id: number
  username: string
  email: string
  display_name: string | null
  avatar_url: string | null
  status: UserStatus
  last_login_at: string | null
  last_login_ip: string | null
  roles: string[]
  role_ids?: number[]
  created_at: string
  updated_at: string
}

export interface UserQuery extends PageParams {
  keyword?: string
  status?: UserStatus | ''
}

export interface UserPayload {
  username: string
  email: string
  password?: string
  display_name?: string
  status?: UserStatus
}

/** 用户分页列表 */
export function listUsers(params: UserQuery): Promise<PageResult<AdminUser>> {
  return request.get<PageResult<AdminUser>>('/users', { params })
}

/** 创建用户 */
export function createUser(data: UserPayload): Promise<AdminUser> {
  return request.post<AdminUser>('/users', data)
}

/** 编辑用户 */
export function updateUser(id: number, data: Partial<UserPayload>): Promise<AdminUser> {
  return request.put<AdminUser>(`/users/${id}`, data)
}

/** 删除用户 */
export function removeUser(id: number): Promise<null> {
  return request.delete<null>(`/users/${id}`)
}

/** 重置密码 */
export function resetUserPassword(id: number, password: string): Promise<null> {
  return request.put<null>(`/users/${id}/password`, { password })
}

/** 分配角色 */
export function assignUserRoles(id: number, role_ids: number[]): Promise<null> {
  return request.put<null>(`/users/${id}/roles`, { role_ids })
}
