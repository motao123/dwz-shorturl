import { ref, shallowRef, type Ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { normalizeListResponse, type ListResponse } from './listResponses'

export interface UseListDataOptions<T> {
  /**
   * 加载函数。必须把「请求参数快照」一并返回（或作为闭包读取），
   * 以便并发请求回来时能丢弃过期结果（后发请求覆盖先发结果的防护）。
   */
  fetcher: () => Promise<ListResponse<T>>
  /** 失败时的提示文案 */
  errorMessage?: string
  /**
   * 加载成功后的回调（如补充筛选项、写回计数）。
   * 只有在结果未被判定为过期时才会执行。
   */
  onLoaded?: (list: T[], total: number) => void
  /** 刷新（已有数据时的重新加载）时是否静默，即不切换 loading 骨架 */
  refreshSilently?: boolean
}

/**
 * 列表数据加载：loading / 数据 / 总数 / 错误 / 空态 / 刷新 / 并发竞态防护。
 *
 * 竞态防护说明：快速切页或连续切换筛选时，先发出的请求可能后返回，
 * 直接把旧数据盖在新数据上（现网隐性缺陷）。这里用自增请求序号丢弃过期响应，
 * 并在 `isStale()` 中暴露给调用方，避免过期响应写回无关状态。
 */
export function useListData<T>(options: UseListDataOptions<T>) {
  const { fetcher, errorMessage = '加载失败', onLoaded, refreshSilently = true } = options

  const loading = ref(false)
  const refreshing = ref(false)
  // shallowRef：列表数据按整体替换（load 时整批换掉），无需深度追踪，
  // 大列表下比 ref 少一层 Proxy 开销。行内编辑请先替换对象再重新赋值。
  const rows = shallowRef<T[]>([]) as Ref<T[]>
  const total = ref(0)
  const error = ref<Error | null>(null)
  const hasLoaded = ref(false)

  /** 请求序号：只有最新一次请求的响应可以写回状态 */
  let latestRequestId = 0

  const empty = ref(true)

  // rows 为 shallowRef，用独立 ref 暴露空态，避免在模板里每次重新计算
  function syncEmpty() {
    empty.value = rows.value.length === 0
  }

  function reset(): void {
    rows.value = []
    total.value = 0
    error.value = null
    syncEmpty()
  }

  /** 当前正在进行的请求是否已过期（调用方用于跳过非列表状态的写回） */
  function isStale(requestId: number): boolean {
    return requestId !== latestRequestId
  }

  /**
   * 加载列表。
   * @returns 当前这次请求是否仍然有效（未被更新的请求取代）
   */
  async function load(): Promise<boolean> {
    const requestId = ++latestRequestId
    const isRefresh = hasLoaded.value && refreshSilently
    if (isRefresh) refreshing.value = true
    else loading.value = true
    error.value = null

    try {
      const res = await fetcher()
      // 过期响应直接丢弃：不写数据、不提示、不关 loading
      if (isStale(requestId)) return false
      const normalized = normalizeListResponse(res)
      rows.value = normalized.list
      total.value = normalized.total
      hasLoaded.value = true
      syncEmpty()
      onLoaded?.(normalized.list, normalized.total)
      return true
    } catch (err) {
      if (isStale(requestId)) return false
      const e = err instanceof Error ? err : new Error(String(err))
      // 先清空数据再记录错误：reset() 会重置 error，顺序不能反
      reset()
      error.value = e
      ElMessage.error(e.message || errorMessage)
      return true
    } finally {
      if (!isStale(requestId)) {
        loading.value = false
        refreshing.value = false
      }
    }
  }

  /** 供弹窗保存、行删除等操作后刷新列表使用（语义等同 load，便于阅读） */
  const reload = load

  return {
    loading,
    refreshing,
    rows,
    total,
    error,
    empty,
    hasLoaded,
    isStale,
    load,
    reload,
    reset
  }
}
