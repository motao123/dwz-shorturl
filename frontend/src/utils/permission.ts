import { useAuthStore } from '@/stores/auth'

/**
 * 判断当前用户是否拥有指定权限点
 * 权限格式：`resource.action`，如 `short_urls.create`
 * super_admin 角色默认放行全部权限
 */
export function hasPermission(perm: string): boolean {
  const auth = useAuthStore()
  if (auth.isSuperAdmin) return true
  return auth.permissions.includes(perm)
}

/** 任一权限命中即通过 */
export function hasAnyPermission(perms: string[]): boolean {
  return perms.some((p) => hasPermission(p))
}
