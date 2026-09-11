import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('element-plus/es/components/message/index', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() }
}))
vi.mock('element-plus/es/components/message-box/index', () => ({
  ElMessageBox: { confirm: vi.fn() }
}))

import { useListPage } from '../useListPage'
import { flushPromises } from './test-utils'

interface Row {
  id: number
  uid: string
  [key: string]: unknown
}

interface Harness {
  urls: string[]
  list: ReturnType<typeof useListPage<Row>>
  fetcherCalls: Record<string, any>[]
}

/**
 * 在一个「伪页面」里使用 useListPage：
 * 与真实列表页一样，把 URL 还原 + 首次加载显式串起来，
 * 从而在没有组件挂载的情况下验证骨架的完整数据流。
 *
 * 注意：URL 中的条件由「列表页在 setup 阶段调用 hydrateFromUrl」负责还原，
 * useListPage 自身只负责在 onMounted 触发首次请求（此处 immediate: false 由测试驱动）。
 */
function createPage(initialSearch = '', overrides: Record<string, unknown> = {}): Harness {
  const urls: string[] = []
  const fetcherCalls: Record<string, any>[] = []
  let url = `https://example.com/admin/short-urls${initialSearch}`

  // 内存版地址栏：既隔离真实 location，也便于断言 URL 同步结果
  const readLocation = () => {
    const u = new URL(url)
    return { search: u.search, pathname: u.pathname, hash: u.hash }
  }
  const writeHistory = (next: string) => {
    urls.push(next)
    url = `https://example.com${next}`
  }

  const list = useListPage<Row>({
    perPage: 20,
    perPageOptions: [10, 20, 50, 100],
    filters: { keyword: '', status: '', sort: 'created_at', order: 'desc' },
    fetcher: async (params) => {
      fetcherCalls.push({ ...params })
      return { list: [{ id: Number(params.page), uid: `u${params.page}` }], total: 100 }
    },
    immediate: false,
    readLocation,
    writeHistory,
    ...(overrides as object)
  })

  // 模拟 onMounted：先还原 URL，再发首次请求
  list.hydrateFromUrl()
  void list.load()
  return { urls, list, fetcherCalls }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useListPage 列表页骨架', () => {
  it('URL 条件还原后首次请求带上筛选（刷新页面筛选不丢）', async () => {
    // 模拟用户带着 ?page=3&per_page=50&keyword=dwz&status=1 打开页面（含日期等复杂条件）
    const { list, fetcherCalls } = createPage('?page=3&per_page=50&keyword=dwz&status=1')
    list.hydrateFromUrl()
    await flushPromises()
    expect(list.page.value).toBe(3)
    expect(list.perPage.value).toBe(50)
    expect(fetcherCalls[0]).toMatchObject({ page: 3, per_page: 50, keyword: 'dwz', status: '1' })
    expect(list.total.value).toBe(100)
    expect(list.rows.value).toHaveLength(1)
  })

  it('筛选变更回到第 1 页并重新请求', async () => {
    const { list, fetcherCalls } = createPage()
    await flushPromises()
    list.setFilter('keyword', 'abc')
    await list.load()
    expect(list.page.value).toBe(1)
    expect(fetcherCalls.at(-1)).toMatchObject({ page: 1, keyword: 'abc' })
  })

  it('search() 语义：重置分页 + 加载', async () => {
    const { list, fetcherCalls } = createPage()
    await flushPromises()
    list.setPage(4)
    await list.load()
    await list.search()
    expect(fetcherCalls.at(-1)).toMatchObject({ page: 1 })
  })

  it('handleReset 恢复到默认筛选（含默认排序）', async () => {
    const { list, fetcherCalls } = createPage()
    await flushPromises()
    list.setFilters({ keyword: 'x', status: '1' }, { resetPage: false })
    list.setFilters({ sort: 'clicks', order: 'asc' }, { resetPage: false })
    list.handleReset()
    await flushPromises()
    expect(fetcherCalls.at(-1)).toMatchObject({ keyword: '', status: '', sort: 'created_at', order: 'desc' })
  })

  it('分页器布局按屏宽收敛，可通过选项覆盖', () => {
    const { list } = createPage()
    expect(list.pagerLayout.value).toBeTruthy()
    const custom = createPage('', { pagerLayout: () => 'prev, pager, next' })
    expect(custom.list.pagerLayout.value).toBe('prev, pager, next')
  })

  it('翻页与筛选变更写回地址栏，空值不出现在 URL 中', async () => {
    const { list, urls } = createPage()
    await flushPromises()
    list.setFilter('keyword', 'dwz')
    list.setPage(2)
    const last = urls.at(-1) ?? ''
    expect(last).toContain('page=2')
    expect(last).toContain('per_page=20')
    expect(last).toContain('keyword=dwz')
    expect(last).not.toContain('status=')
  })

  it('批量操作与列表共享选中状态，成功后清空勾选', async () => {
    const { list } = createPage()
    await flushPromises()
    list.setSelected([{ id: 1, uid: 'a' }])
    expect(list.hasSelection.value).toBe(true)
    const executed: number[][] = []
    await list.execute(async (ids) => {
      executed.push(ids)
      return {}
    })
    expect(executed).toEqual([[1]])
    expect(list.hasSelection.value).toBe(false)
  })

  it('加载失败时清空列表并给出提示', async () => {
    const { list } = createPage('', { fetcher: async () => Promise.reject(new Error('boom')) })
    await flushPromises()
    expect(list.rows.value).toEqual([])
    expect(list.error.value?.message).toBe('boom')
  })
})
