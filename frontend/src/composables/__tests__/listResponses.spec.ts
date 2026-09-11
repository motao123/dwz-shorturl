import { describe, expect, it } from 'vitest'
import { normalizeArrayResponse, normalizeListResponse } from '../listResponses'

describe('normalizeListResponse', () => {
  it('裸数组：total 取数组长度（与迁移前各页行为一致）', () => {
    const res = normalizeListResponse([{ id: 1 }, { id: 2 }])
    expect(res.list).toHaveLength(2)
    expect(res.total).toBe(2)
  })

  it('分页包裹：透传 list/total', () => {
    const res = normalizeListResponse({ list: [{ id: 1 }], total: 42, page: 1, per_page: 20 })
    expect(res.list).toEqual([{ id: 1 }])
    expect(res.total).toBe(42)
  })

  it('空响应兜底为空列表', () => {
    expect(normalizeListResponse(null)).toEqual({ list: [], total: 0 })
    expect(normalizeListResponse(undefined)).toEqual({ list: [], total: 0 })
  })

  it('分页包裹缺字段时兜底', () => {
    const res = normalizeListResponse({} as never)
    expect(res).toEqual({ list: [], total: 0 })
  })

  it('normalizeArrayResponse 只取列表', () => {
    expect(normalizeArrayResponse([{ id: 1 }])).toEqual([{ id: 1 }])
    expect(normalizeArrayResponse({ list: [{ id: 2 }], total: 1, page: 1, per_page: 20 })).toEqual([{ id: 2 }])
  })
})
