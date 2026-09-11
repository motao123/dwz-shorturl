import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('element-plus/es/components/message/index', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() }
}))
vi.mock('element-plus/es/components/message-box/index', () => ({
  ElMessageBox: { confirm: vi.fn() }
}))

import { useListPage } from '../useListPage'
import { flushPromises } from './test-utils'

/**
 * 迁移后的页面行为回归：用真实页面的参数组装方式（buildParams 派生字段）
 * 验证「回收站切换 / 日期范围 / 排序」三处最容易改坏的行为与迁移前一致。
 */
interface ShortUrl {
  id: number
  uid: string
  [key: string]: unknown
}

function createShortUrlPage(initialSearch = '') {
  const fetcherCalls: Record<string, any>[] = []
  let url = `https://example.com/admin/short-urls${initialSearch}`
  const page = useListPage<ShortUrl>({
    perPage: 20,
    perPageOptions: [10, 20, 50, 100],
    filters: {
      keyword: '',
      status: '',
      category_id: '',
      date_start: '',
      date_end: '',
      sort: 'created_at',
      order: 'desc',
      include_deleted: 0 as 0 | 1
    },
    fetcher: async (params) => {
      fetcherCalls.push({ ...params })
      return { list: [], total: 0 }
    },
    immediate: false,
    readLocation: () => {
      const u = new URL(url)
      return { search: u.search, pathname: u.pathname, hash: u.hash }
    },
    writeHistory: (next) => {
      url = `https://example.com${next}`
    }
  })

  const showTrash = () => page.filters.value.include_deleted === 1
  // 与 ShortUrlList.vue 中的 buildParams 保持同一逻辑
  const buildParams = (params: Record<string, unknown>) => {
    const next = { ...params } as Record<string, unknown>
    next.include_deleted = showTrash() ? 1 : 0
    next.date_from = page.filters.value.date_start || ''
    next.date_to = page.filters.value.date_end || ''
    return next
  }

  return { page, fetcherCalls, buildParams, showTrash, getUrl: () => url }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('短链列表页：关键行为回归', () => {
  it('回收站切换：include_deleted 由 0 → 1 并回到第 1 页', async () => {
    const { page, fetcherCalls, buildParams, showTrash } = createShortUrlPage()
    page.hydrateFromUrl()
    await page.load()
    expect(buildParams(page.buildParams()).include_deleted).toBe(0)

    page.setFilters({ include_deleted: showTrash() ? 0 : 1 })
    await page.load()
    expect(fetcherCalls.at(-1)).toMatchObject({ include_deleted: 1, page: 1 })
  })

  it('排序变更不重置页码', async () => {
    const { page, fetcherCalls, buildParams } = createShortUrlPage()
    page.hydrateFromUrl()
    await page.load()
    page.setPage(3)
    await page.load()
    page.setFilters({ sort: 'clicks', order: 'asc' }, { resetPage: false })
    await page.load()
    expect(fetcherCalls.at(-1)).toMatchObject({ page: 3, sort: 'clicks', order: 'asc' })
    expect(buildParams(page.buildParams()).sort).toBe('clicks')
  })

  it('重置后回到第 1 页且恢复默认排序', async () => {
    const { page, fetcherCalls } = createShortUrlPage()
    page.hydrateFromUrl()
    await page.load()
    page.setPage(4)
    page.setFilters({ keyword: 'x', status: '1', include_deleted: 1 })
    page.handleReset()
    await flushPromises()
    expect(fetcherCalls.at(-1)).toMatchObject({
      page: 1,
      keyword: '',
      status: '',
      sort: 'created_at',
      order: 'desc',
      include_deleted: 0
    })
  })

  it('日期范围写入 URL 并可在刷新后还原', async () => {
    const { page, getUrl } = createShortUrlPage()
    page.hydrateFromUrl()
    await page.load()
    page.setFilters({ date_start: '2026-01-01', date_end: '2026-01-31' })
    expect(getUrl()).toContain('date_start=2026-01-01')
    expect(getUrl()).toContain('date_end=2026-01-31')

    const reopened = createShortUrlPage('?page=2&date_start=2026-01-01&date_end=2026-01-31&include_deleted=1')
    reopened.page.hydrateFromUrl()
    await reopened.page.load()
    expect(reopened.page.page.value).toBe(2)
    expect(reopened.showTrash()).toBe(true)
    expect(reopened.buildParams(reopened.page.buildParams())).toMatchObject({
      date_from: '2026-01-01',
      date_to: '2026-01-31',
      include_deleted: 1
    })
  })
})

describe('审计日志页：日期范围回归', () => {
  it('date_start/date_end 入 URL，请求侧发 date_from/date_to', async () => {
    const fetcherCalls: Record<string, any>[] = []
    let url = 'https://example.com/admin/audit-logs'
    const page = useListPage<{ id: number }>({
      perPage: 20,
      perPageOptions: [20, 50, 100],
      filters: { user_id: '', action: '', date_start: '', date_end: '' },
      fetcher: async (params) => {
        const next = { ...params } as Record<string, unknown>
        next.date_from = params.date_start
        next.date_to = params.date_end
        delete next.date_start
        delete next.date_end
        fetcherCalls.push(next)
        return { list: [], total: 0 }
      },
      immediate: false,
      readLocation: () => {
        const u = new URL(url)
        return { search: u.search, pathname: u.pathname, hash: u.hash }
      },
      writeHistory: (next) => {
        url = `https://example.com${next}`
      }
    })

    page.hydrateFromUrl()
    page.setFilters({ date_start: '2026-02-01', date_end: '2026-02-28' })
    await page.load()
    expect(fetcherCalls.at(-1)).toMatchObject({
      page: 1,
      date_from: '2026-02-01',
      date_to: '2026-02-28'
    })
    expect(fetcherCalls.at(-1)).not.toHaveProperty('date_start')
    expect(url).toContain('date_start=2026-02-01')
  })
})
