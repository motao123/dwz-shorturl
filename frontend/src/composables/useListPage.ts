import { computed, onMounted, type Ref } from 'vue'
import { BREAKPOINT_TABLET, useResponsive } from './useResponsive'
import { useListQuery, type QueryScalar, type UseListQueryOptions } from './useListQuery'
import { useListData } from './useListData'
import { useBatchAction } from './useBatchAction'
import type { ListResponse } from './listResponses'

/**
 * 列表页统一骨架：查询条件 + 数据加载 + 批量操作 + 响应式分页器。
 *
 * 组合 `useListQuery` / `useListData` / `useBatchAction`，并统一
 * `useResponsive`（PR #12 引入）的接入方式：全站列表页共用同一套分页器布局收敛规则。
 */
export interface UseListPageOptions<T> {
  perPage?: number
  perPageOptions?: number[]
  filters?: Record<string, QueryScalar>
  syncUrl?: boolean
  /** 列表接口调用，接收 `useListQuery().buildParams()` 的结果 */
  fetcher: (params: Record<string, any>) => Promise<ListResponse<T>>
  errorMessage?: string
  onLoaded?: (list: T[], total: number) => void
  rowKey?: string
  emptySelectionMessage?: string
  /** 初始化时是否自动加载（含从 URL 还原筛选），默认 true */
  immediate?: boolean
  /** 分页器布局；缺省按屏宽收敛（窄屏去掉 sizes/jumper，避免溢出） */
  pagerLayout?: (ctx: { isMobile: boolean; isTablet: boolean }) => string
  /** 地址栏读写入口，透传给 useListQuery（默认 window.location / history） */
  readLocation?: UseListQueryOptions['readLocation']
  writeHistory?: UseListQueryOptions['writeHistory']
}

export function useListPage<T extends object>(options: UseListPageOptions<T>) {
  const query = useListQuery({
    perPage: options.perPage,
    perPageOptions: options.perPageOptions,
    filters: options.filters,
    readLocation: options.readLocation,
    writeHistory: options.writeHistory
  })
  if (options.syncUrl === false) {
    // 需要关闭同步时，由页面直接组合 useListQuery/useListData，避免此处维护两套分支
    throw new Error('useListPage 默认开启 URL 同步；如需关闭请直接组合 useListQuery + useListData')
  }

  const data = useListData<T>({
    fetcher: () => options.fetcher(query.buildParams()),
    errorMessage: options.errorMessage,
    onLoaded: options.onLoaded
  })

  const batch = useBatchAction<T>({
    rowKey: (options.rowKey ?? 'id') as keyof T & string,
    emptySelectionMessage: options.emptySelectionMessage
  })

  const { isMobile, isTablet } = useResponsive()

  const pagerLayout = computed(() => {
    if (options.pagerLayout) return options.pagerLayout({ isMobile: isMobile.value, isTablet: isTablet.value })
    if (isMobile.value) return 'prev, pager, next'
    if (isTablet.value) return 'total, prev, pager, next'
    return 'total, sizes, prev, pager, next, jumper'
  })

  /** 变更筛选 / 翻页后统一走这里，避免各页面自己记「要不要重置分页」 */
  async function search(): Promise<void> {
    query.setPage(1)
    await data.load()
  }

  async function reload(): Promise<void> {
    await data.load()
  }

  function handlePageChange(page: number): void {
    query.setPage(page)
    void data.load()
  }

  function handleSizeChange(size: number): void {
    query.setPerPage(size)
    void data.load()
  }

  function handleReset(): void {
    query.resetFilters()
    void search()
  }

  if (options.immediate !== false) {
    onMounted(() => {
      // 挂载时先从地址栏还原（刷新/分享链接后筛选条件不丢），再按条件加载
      query.hydrateFromUrl()
      void data.load()
    })
  }

  return {
    ...query,
    ...data,
    ...batch,
    isMobile: isMobile as Ref<boolean>,
    isTablet: isTablet as Ref<boolean>,
    breakpointTablet: BREAKPOINT_TABLET,
    pagerLayout,
    search,
    reload,
    handlePageChange,
    handleSizeChange,
    handleReset
  }
}
