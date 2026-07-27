import request from './request'

export interface Role {
  id: number
  name: string
  display_name: string
  description: string | null
  is_system: 0 | 1
  permissions?: string[]
  created_at: string
  updated_at: string
}

export interface RolePayload {
  name: string
  display_name: string
  description?: string
}

export interface Permission {
  id: number
  resource: string
  action: string
  description: string | null
}

/** 角色列表 */
export function listRoles(): Promise<Role[]> {
  return request.get<Role[]>('/roles')
}

/** 创建角色 */
export function createRole(data: RolePayload): Promise<Role> {
  return request.post<Role>('/roles', data)
}

/** 编辑角色 */
export function updateRole(id: number, data: Partial<RolePayload>): Promise<Role> {
  return request.put<Role>(`/roles/${id}`, data)
}

/** 删除角色 */
export function removeRole(id: number): Promise<null> {
  return request.delete<null>(`/roles/${id}`)
}

/** 设置角色权限（permissions 为 resource.action 数组） */
export function setRolePermissions(id: number, permissions: string[]): Promise<null> {
  return request.put<null>(`/roles/${id}/permissions`, { permissions })
}

/** 获取全部权限定义 */
export function getAllPermissions(): Promise<Permission[]> {
  return request.get<Permission[]>('/permissions')
}
