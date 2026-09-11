import { computed, ref, shallowRef, type Ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'

/**
 * 公共跳转库（wjoy_log）同步失败的统一提示契约。
 * 后台本地操作已成功、但公共库同步失败时，链接可能仍可通过 PHP 路径访问，
 * 必须明确告知用户，避免「界面已删、链接仍可访问」的误判。
 * 契约见 PR #12/#14，此前在多个页面各复制一份，改契约需要多点同步。
 */
export interface SyncFailurePayload {
  public_sync_failed?: boolean
  sync_failed_uids?: string[]
  warning?: string
}

export interface SyncFailureMessages {
  /** 单条操作场景 */
  single?: string
  /** 批量操作场景 */
  batch?: string
}

const DEFAULT_MESSAGES: Required<SyncFailureMessages> = {
  single: '本地已操作成功，但公共库(wjoy_log)同步失败，链接可能仍可通过 PHP 路径访问（系统会在 30 分钟内自动补偿）。',
  batch: '本地已操作成功，但部分公共库(wjoy_log)同步失败，这些短码可能仍可通过 PHP 路径访问，系统将在 30 分钟内自动补偿。'
}

function hasSyncFailure(res: SyncFailurePayload | null | undefined): boolean {
  return Boolean(res && res.public_sync_failed)
}

/** 单条操作：命中同步失败契约时提示 warning，否则提示 success */
export function notifySyncResult(
  res: SyncFailurePayload | null | undefined,
  successMessage: string,
  messages: SyncFailureMessages = {}
): void {
  if (!hasSyncFailure(res)) {
    ElMessage.success(successMessage)
    return
  }
  const text = messages.single ?? DEFAULT_MESSAGES.single
  ElMessage.warning({
    message: `${text}${res?.warning ?? ''}`,
    duration: 6000
  })
}

/**
 * 批量操作：命中同步失败契约时，把失败 UID 一并列出，方便用户核对。
 * `count` 用于成功文案（通常为选中条数或接口返回的处理条数）。
 */
export function notifyBatchSyncResult(
  res: SyncFailurePayload | null | undefined,
  count: number,
  messages: SyncFailureMessages = {}
): void {
  if (!hasSyncFailure(res)) {
    ElMessage.success(`已处理 ${count} 条`)
    return
  }
  const uids = res?.sync_failed_uids ?? []
  const text = messages.batch ?? DEFAULT_MESSAGES.batch
  ElMessage.warning({
    message: `${text}${uids.length ? `（${uids.join('、')}）` : ''}`,
    duration: 8000
  })
}

export interface BatchConfirmOptions {
  /** 确认框标题 */
  title?: string
  /** 确认文案；可传函数以带上选中条数 */
  message: string | ((ids: number[]) => string)
  confirmButtonText?: string
  cancelButtonText?: string
}

export interface UseBatchActionOptions<T> {
  /** 行主键，默认 `id` */
  rowKey?: keyof T & string
  /** 未勾选任何行时的提示（传空串可关闭提示） */
  emptySelectionMessage?: string
  /** 批量执行期间是否在成功后清空勾选（默认 true，避免重复提交已删除的行） */
  clearAfterSuccess?: boolean
}

/**
 * 批量选择 + 执行 + 结果提示。
 *
 * 与 `useListData` 配套：执行成功后调用方自行 `load()` 刷新列表，
 * 或把刷新动作交给调用方注入，保持「谁拥有请求谁负责刷新」的边界。
 */
export function useBatchAction<T extends object>(options: UseBatchActionOptions<T> = {}) {
  const { rowKey = 'id' as keyof T & string, emptySelectionMessage = '请先勾选要操作的数据', clearAfterSuccess = true } = options

  const selected = shallowRef<T[]>([]) as Ref<T[]>
  const executing = ref(false)

  const selectedIds = computed<number[]>(() =>
    selected.value.map((row) => Number(row[rowKey])).filter((id) => Number.isFinite(id))
  )
  const hasSelection = computed(() => selected.value.length > 0)
  const selectedCount = computed(() => selected.value.length)

  function setSelected(rows: T[]): void {
    selected.value = rows ?? []
  }

  function clearSelection(): void {
    selected.value = []
  }

  function ensureSelection(): boolean {
    if (hasSelection.value) return true
    if (emptySelectionMessage) ElMessage.warning(emptySelectionMessage)
    return false
  }

  async function confirm(confirmOptions: BatchConfirmOptions): Promise<boolean> {
    const message =
      typeof confirmOptions.message === 'function' ? confirmOptions.message(selectedIds.value) : confirmOptions.message
    try {
      await ElMessageBox.confirm(message, confirmOptions.title ?? '操作确认', {
        confirmButtonText: confirmOptions.confirmButtonText ?? '确定',
        cancelButtonText: confirmOptions.cancelButtonText ?? '取消',
        type: 'warning'
      })
      return true
    } catch {
      return false
    }
  }

  /**
   * 执行批量操作。
   * @param run 实际请求体，接收选中行 id 数组
   * @param config 确认框 / 成功文案 / 同步失败文案 / 失败文案
   */
  async function execute<D>(
    run: (ids: number[]) => Promise<D>,
    config: {
      confirm?: BatchConfirmOptions | null
      /** 成功提示，默认「已处理 N 条」 */
      successMessage?: string | ((count: number) => string)
      syncMessages?: SyncFailureMessages
      errorMessage?: string
      /** 是否把接口返回的 public_sync_failed 交给统一契约提示（默认 true） */
      notifySync?: boolean
    } = {}
  ): Promise<boolean> {
    if (!ensureSelection()) return false
    if (config.confirm) {
      const ok = await confirm(config.confirm)
      if (!ok) return false
    }

    executing.value = true
    try {
      const ids = selectedIds.value
      const res = await run(ids)
      if (config.notifySync === false) {
        const message =
          typeof config.successMessage === 'function'
            ? config.successMessage(ids.length)
            : (config.successMessage ?? `已处理 ${ids.length} 条`)
        ElMessage.success(message)
      } else if (typeof config.successMessage === 'string') {
        notifySyncResult(res as SyncFailurePayload, config.successMessage, config.syncMessages)
      } else {
        notifyBatchSyncResult(res as SyncFailurePayload, ids.length, config.syncMessages)
      }
      if (clearAfterSuccess) clearSelection()
      return true
    } catch (err) {
      ElMessage.error(err instanceof Error ? err.message : (config.errorMessage ?? '批量操作失败'))
      return false
    } finally {
      executing.value = false
    }
  }

  return {
    selected,
    selectedIds,
    selectedCount,
    hasSelection,
    executing,
    setSelected,
    clearSelection,
    ensureSelection,
    confirm,
    execute
  }
}
