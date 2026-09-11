import type { PageResult } from '@/api/request'

/**
 * 列表接口响应归一化。
 *
 * 后端部分端点历史上返回裸数组（无分页包裹），另一些返回 `{ list, total }`。
 * 抽取前每个页面各写一遍 `Array.isArray(res) ? ... : ...` 判断，行为还略有差异
 * （有的取 `res.length` 当 total，有的回退 0）。这里统一为一处，
 * 语义与迁移前保持一致：裸数组时 total = 数组长度。
 */
export interface NormalizedList<T> {
  list: T[]
  total: number
}

/** 分页列表接口的响应类型：分页包裹（page/per_page 可缺省）或裸数组 */
export type ListResponse<T> = PageResult<T> | ArrayLike<T> | { list: T[]; total: number } | null | undefined

export function normalizeListResponse<T>(res: ListResponse<T>): NormalizedList<T> {
  if (Array.isArray(res)) {
    return { list: res, total: res.length }
  }
  if (!res) {
    return { list: [], total: 0 }
  }
  const wrapped = res as { list?: T[]; total?: number }
  return { list: wrapped.list ?? [], total: wrapped.total ?? 0 }
}

/** 仅需要列表数据的接口（角色、权限、域名等小数据集） */
export function normalizeArrayResponse<T>(res: ListResponse<T>): T[] {
  return normalizeListResponse(res).list
}
