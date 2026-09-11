import { computed, isRef, ref, type Ref } from 'vue'

/** URL query 中被忽略（不参与双向同步）的保留参数 */
const RESERVED_QUERY_KEYS = ['page', 'per_page', 'page_size']

export type QueryScalar = string | number | boolean | null | undefined | ''

export interface UseListQueryOptions {
  /** 默认每页条数，切换筛选/搜索后回到第 1 页 */
  perPage?: number
  /** 每页条数候选值；为空表示该列表不提供「每页条数」切换 */
  perPageOptions?: number[]
  /**
   * 参与筛选的字段默认值。键名即 URL query 参数名，
   * 键名必须与列表接口的 query 参数名一致，才能直接透传给请求。
   */
  filters?: Record<string, QueryScalar>
  /**
   * 是否把分页与筛选同步到地址栏 query（默认 true）。
   * 关闭后仅在内存中维护，不在 URL 留下痕迹（如敏感筛选条件）。
   */
  syncUrl?: boolean
  /** 注入的读写入口，便于单测；默认使用 window.location / history */
  readLocation?: () => { search: string; pathname: string; hash: string }
  writeHistory?: (url: string, mode: 'replace' | 'push') => void
}

/** 地址栏 query 字符串 → 扁平对象（自动做类型还原） */
function parseUrlQuery(search: string): Record<string, string> {
  const out: Record<string, string> = {}
  if (!search) return out
  const params = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search)
  params.forEach((value, key) => {
    out[key] = value
  })
  return out
}

export function defaultReadLocation() {
  if (typeof window === 'undefined') return { search: '', pathname: '', hash: '' }
  return { search: window.location.search, pathname: window.location.pathname, hash: window.location.hash }
}

export function defaultWriteHistory(url: string, mode: 'replace' | 'push') {
  if (typeof window === 'undefined' || !window.history) return
  if (mode === 'push') window.history.pushState(window.history.state, '', url)
  else window.history.replaceState(window.history.state, '', url)
}

/**
 * 地址栏读写入口。单测通过替换这两个槽位注入内存实现（见 useListPage.spec.ts），
 * 生产运行态下与直接调用 window.location / history 完全一致。
 */
export const DEFAULT_READ_LOCATION = { read: defaultReadLocation }
export const DEFAULT_WRITE_HISTORY = { write: defaultWriteHistory }

/**
 * 列表页查询条件（分页 + 筛选）→ 请求参数，并与地址栏 query 双向同步。
 *
 * 使用约定：
 * - 筛选字段名必须与后端 query 参数名一致（如 `keyword`、`date_from`）；
 * - 变更筛选请用 `setFilter` / `resetFilters`，它们会自动回到第 1 页；
 * - 请求参数用 `buildParams()` 获取（同步完日期格式化等派生字段之后）。
 *
 * URL 约定（详见 docs/frontend-list-composables.md）：
 * - `page`（从 1 开始）、`per_page`
 * - 筛选字段直接以字段名入参；空值一律从 URL 中移除
 * - 字符串 `''`、`undefined`、`null` 视为「未筛选」，不会写入 URL
 */
export function useListQuery(options: UseListQueryOptions = {}) {
  const {
    perPage = 20,
    perPageOptions = [],
    filters = {},
    syncUrl = true,
    // 通过槽位（而非直接默认值）解析：单测可在调用前替换，
    // 生产运行态下与直接调用 window.location / history 等价
    readLocation = DEFAULT_READ_LOCATION.read,
    writeHistory = DEFAULT_WRITE_HISTORY.write
  } = options

  const filterDefaults: Record<string, QueryScalar> = { ...filters }

  const page = ref(1)
  const perPageValue = ref(perPage)
  const filterState = ref<Record<string, QueryScalar>>({ ...filterDefaults })

  function isEmpty(value: QueryScalar): boolean {
    return value === '' || value === null || value === undefined
  }

  /**
   * 按默认值的类型还原 URL 中的字符串。
   * 数字型字段必须还原为数字，否则 el-select 的 `:value="1"` 与字符串 `'1'`
   * 永远不相等，回填后的下拉框会显示空白（页面看起来「筛选丢了」）。
   */
  function coerce(key: string, raw: string): QueryScalar {
    const seed = filterDefaults[key]
    // 数字型还原为数字；非数字的数值型筛选（如 `status: '' | 1` 这种联合声明，
    // 运行期默认值为字符串）按字面量回填，只有纯 number 默认值才做转换
    if (typeof seed === 'number') {
      const n = Number(raw)
      return Number.isFinite(n) ? n : seed
    }
    if (typeof seed === 'boolean') return raw === 'true' || raw === '1'
    return raw
  }

  function toPositiveInt(raw: string | undefined, fallback: number): number {
    const n = Number(raw)
    return Number.isFinite(n) && n > 0 ? Math.floor(n) : fallback
  }

  /** 从地址栏还原状态（页面初始化时调用一次） */
  function hydrateFromUrl(): void {
    if (!syncUrl) return
    const raw = parseUrlQuery(readLocation().search)

    page.value = toPositiveInt(raw.page, 1)
    if (perPageOptions.length) {
      const size = toPositiveInt(raw.per_page ?? raw.page_size, perPage)
      perPageValue.value = perPageOptions.includes(size) ? size : perPage
    }

    const next: Record<string, QueryScalar> = { ...filterDefaults }
    for (const key of Object.keys(filterDefaults)) {
      const value = raw[key]
      if (value === undefined || value === '') continue
      next[key] = coerce(key, value)
    }
    filterState.value = next
  }

  function buildQuery(): string {
    const params = new URLSearchParams()
    params.set('page', String(page.value))
    params.set('per_page', String(perPageValue.value))
    for (const [key, value] of Object.entries(filterState.value)) {
      if (RESERVED_QUERY_KEYS.includes(key) || isEmpty(value)) continue
      params.set(key, String(value))
    }
    return params.toString()
  }

  /** 把当前状态写回地址栏（replace 默认，避免每次翻页都堆历史记录） */
  function syncToUrl(mode: 'replace' | 'push' = 'replace'): void {
    if (!syncUrl) return
    const query = buildQuery()
    const loc = readLocation()
    const base = typeof window === 'undefined' ? '/' : loc.pathname
    writeHistory(`${base}${query ? `?${query}` : ''}${loc.hash ?? ''}`, mode)
  }

  /**
   * 当前状态对应的请求参数（不含派生字段，如 date_from/date_to，请在页面侧合并）。
   *
   * 空值统一输出空串而不是省略字段：迁移前各页就是 `{ ...query }` 透传，
   * 改成省略字段会改变请求形态（后端对「未传」与「空串」的处理可能不同）。
   */
  function buildParams<T extends Record<string, unknown> = Record<string, unknown>>(): T {
    const params: Record<string, unknown> = {}
    for (const [key, value] of Object.entries(filterState.value)) {
      // 保留参数由分页状态维护，避免 filterRef('page') 把分页写坏
      if (RESERVED_QUERY_KEYS.includes(key)) continue
      params[key] = isEmpty(value) ? '' : value
    }
    params.page = page.value
    params.per_page = perPageValue.value
    return params as T
  }

  function setPage(next: number): void {
    page.value = toPositiveInt(String(next), 1)
    syncToUrl()
  }

  function setPerPage(next: number): void {
    perPageValue.value = toPositiveInt(String(next), perPage)
    // 每页条数变化后当前页可能越界，统一回到第 1 页（与迁移前各页行为一致）
    page.value = 1
    syncToUrl()
  }

  function resolve(value: QueryScalar | Ref<QueryScalar>): QueryScalar {
    return isRef(value) ? value.value : value
  }

  /** 变更单个筛选条件并回到第 1 页 */
  function setFilter<K extends string>(key: K, raw: QueryScalar | Ref<QueryScalar>, opts: { resetPage?: boolean } = {}): void {
    filterState.value = { ...filterState.value, [key]: resolve(raw) }
    if (opts.resetPage !== false) page.value = 1
    syncToUrl()
  }

  /** 批量变更筛选条件（一次触发，避免多次 reset 与同步） */
  function setFilters(patch: Record<string, QueryScalar | Ref<QueryScalar>>, opts: { resetPage?: boolean } = {}): void {
    const next = { ...filterState.value }
    for (const [key, value] of Object.entries(patch)) next[key] = resolve(value)
    filterState.value = next
    if (opts.resetPage !== false) page.value = 1
    syncToUrl()
  }

  /** 重置全部筛选条件（不重置分页，调用方通常随后 `setPage(1)`） */
  function resetFilters(): void {
    filterState.value = { ...filterDefaults }
    syncToUrl()
  }

  // 单个筛选字段的读写 ref，模板里可直接 v-model（变更后自动回第 1 页并同步 URL）
  const filterRefs = new Map<string, Ref<unknown>>()
  function filterRef<T = QueryScalar>(key: string): Ref<T> {
    const cached = filterRefs.get(key)
    if (cached) return cached as Ref<T>
    const r = computed<QueryScalar>({
      get: () => filterState.value[key],
      set: (value) => setFilter(key, value)
    }) as unknown as Ref<T>
    filterRefs.set(key, r)
    return r as Ref<T>
  }

  /** 具有非默认值的筛选字段数量（用于「已筛选 N 项」提示） */
  const activeFilterCount = computed(
    () => Object.keys(filterDefaults).filter((key) => !isEmpty(filterState.value[key])).length
  )

  return {
    page,
    perPage: perPageValue,
    filters: filterState,
    perPageOptions,
    setPage,
    setPerPage,
    setFilter,
    setFilters,
    resetFilters,
    filterRef,
    buildParams,
    buildQuery,
    hydrateFromUrl,
    syncToUrl,
    activeFilterCount
  }
}
