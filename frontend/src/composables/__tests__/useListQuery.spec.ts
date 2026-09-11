import { describe, expect, it } from 'vitest'
import { useListQuery } from '../useListQuery'

function createHarness(initialSearch: string) {
  let url = `https://example.com/admin/users${initialSearch}`
  const writes: string[] = []
  const query = useListQuery({
    perPage: 20,
    perPageOptions: [10, 20, 50],
    // status 为数值型筛选（默认值为数字），URL 回填时必须保持数字语义
    filters: { keyword: '', status: '' as number | '', date_from: '', date_to: '' },
    readLocation: () => {
      const u = new URL(url)
      return { search: u.search, pathname: u.pathname, hash: u.hash }
    },
    writeHistory: (next) => {
      writes.push(next)
      url = `https://example.com${next}`
    }
  })
  return { query, writes, getUrl: () => url }
}

describe('useListQuery', () => {
  it('默认值：第 1 页，每页 20，无筛选', () => {
    const { query } = createHarness('')
    expect(query.page.value).toBe(1)
    expect(query.perPage.value).toBe(20)
    expect(query.activeFilterCount.value).toBe(0)
  })

  it('buildParams 输出接口参数，空筛选保持为空串（不改变后端语义）', () => {
    const { query } = createHarness('')
    expect(query.buildParams()).toEqual({
      page: 1,
      per_page: 20,
      keyword: '',
      status: '',
      date_from: '',
      date_to: ''
    })
  })

  it('setFilter 变更筛选并回到第 1 页', () => {
    const { query } = createHarness('')
    query.setPage(5)
    query.setFilter('keyword', 'dwz')
    expect(query.page.value).toBe(1)
    expect(query.buildParams().keyword).toBe('dwz')
    expect(query.activeFilterCount.value).toBe(1)
  })

  it('setFilters 一次性写入多个筛选', () => {
    const { query } = createHarness('')
    query.setFilters({ keyword: 'a', status: 1 })
    expect(query.buildParams()).toMatchObject({ page: 1, keyword: 'a', status: 1 })
  })

  it('resetFilters 恢复默认值，但不清空筛选之外的字段', () => {
    const { query } = createHarness('')
    query.setFilters({ keyword: 'a', status: 1 })
    query.resetFilters()
    expect(query.buildParams()).toMatchObject({ keyword: '', status: '' })
  })

  it('URL 同步：筛选/分页写入地址栏，空值被移除', () => {
    const { query, getUrl } = createHarness('')
    query.setFilter('keyword', 'dwz')
    query.setPage(3)
    expect(getUrl()).toContain('page=3')
    expect(getUrl()).toContain('per_page=20')
    expect(getUrl()).toContain('keyword=dwz')
    expect(getUrl()).not.toContain('status=')
  })

  it('URL 还原：从地址栏 hydrate 出分页与筛选', () => {
    const { query } = createHarness('?page=4&per_page=50&keyword=dwz&status=2')
    query.hydrateFromUrl()
    expect(query.page.value).toBe(4)
    expect(query.perPage.value).toBe(50)
    expect(query.buildParams().keyword).toBe('dwz')
    expect(query.buildParams().status).toBe('2')
    expect(query.activeFilterCount.value).toBe(2)
  })

  it('URL 还原：数值型筛选（默认值为数字）保持数字语义，否则下拉框回填会失效', () => {
    const location = '?page=1&status=2'
    const query = useListQuery({
      perPage: 20,
      filters: { status: 0 as number | '' },
      readLocation: () => ({ search: location, pathname: '/admin/users', hash: '' }),
      writeHistory: () => {}
    })
    query.hydrateFromUrl()
    expect(query.filters.value.status).toBe(2)
    expect(query.buildParams().status).toBe(2)
  })

  it('URL 还原：数值型筛选为非法值时回退默认值', () => {
    const query = useListQuery({
      filters: { status: 0 as number | '' },
      readLocation: () => ({ search: '?status=abc', pathname: '/x', hash: '' }),
      writeHistory: () => {}
    })
    query.hydrateFromUrl()
    expect(query.filters.value.status).toBe(0)
  })

  it('URL 还原：非法分页参数回退默认值', () => {
    const { query } = createHarness('?page=-1&per_page=999')
    query.hydrateFromUrl()
    expect(query.page.value).toBe(1)
    expect(query.perPage.value).toBe(20)
  })

  it('per_page 别名兼容 page_size', () => {
    const { query } = createHarness('?page_size=10')
    query.hydrateFromUrl()
    expect(query.perPage.value).toBe(10)
  })

  it('setPerPage 变更每页条数后回到第 1 页', () => {
    const { query } = createHarness('')
    query.setPage(7)
    query.setPerPage(50)
    expect(query.perPage.value).toBe(50)
    expect(query.page.value).toBe(1)
  })

  it('filterRef 支持 v-model 式读写', () => {
    const { query } = createHarness('')
    const status = query.filterRef('status')
    status.value = 1
    expect(query.buildParams().status).toBe(1)
    expect(query.page.value).toBe(1)
  })

  it('保留参数不会被筛选字段覆盖（page/per_page 由分页状态维护）', () => {
    const { query } = createHarness('')
    query.setFilter('page', 99)
    expect(query.buildParams().page).toBe(1)
  })
})
