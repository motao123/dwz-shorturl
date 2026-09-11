import { ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { notifySyncResult, type SyncFailureMessages, type SyncFailurePayload } from './useBatchAction'

export interface ConfirmOptions {
  title?: string
  message: string
  confirmButtonText?: string
  cancelButtonText?: string
}

/**
 * 二次确认。取消（用户点「取消」/关闭）返回 false，
 * 不抛异常，调用方 `if (!(await confirmAction(...))) return` 即可。
 */
export async function confirmAction(options: ConfirmOptions): Promise<boolean> {
  try {
    await ElMessageBox.confirm(options.message, options.title ?? '操作确认', {
      confirmButtonText: options.confirmButtonText ?? '确定',
      cancelButtonText: options.cancelButtonText ?? '取消',
      type: 'warning'
    })
    return true
  } catch {
    return false
  }
}

export interface UseRowActionOptions {
  /** 操作成功后刷新列表（通常是 `useListData().load`） */
  onSuccess?: () => void | Promise<void>
}

/**
 * 行级操作（删除 / 恢复 / 归档等）的统一封装：
 * 二次确认 → 执行 → 结果提示（含公共库同步失败契约）→ 刷新 → 错误兜底。
 *
 * 此前每个页面的 `handleRemove` 都重复这套流程，且同步失败提示文案各写一遍。
 */
export function useRowAction(options: UseRowActionOptions = {}) {
  const acting = ref(false)

  async function run<D>(
    action: () => Promise<D>,
    config: {
      confirm?: ConfirmOptions | null
      successMessage?: string
      syncMessages?: SyncFailureMessages
      errorMessage?: string
      /** 是否把接口返回的 public_sync_failed 交给统一契约提示（默认 true） */
      notifySync?: boolean
      /** 是否在成功后刷新列表（默认 true） */
      refresh?: boolean
    } = {}
  ): Promise<boolean> {
    if (config.confirm) {
      const ok = await confirmAction(config.confirm)
      if (!ok) return false
    }

    acting.value = true
    try {
      const res = await action()
      if (config.notifySync === false) {
        if (config.successMessage) ElMessage.success(config.successMessage)
      } else {
        notifySyncResult(res as SyncFailurePayload, config.successMessage ?? '操作成功', config.syncMessages)
      }
      if (config.refresh !== false && options.onSuccess) await options.onSuccess()
      return true
    } catch (err) {
      ElMessage.error(err instanceof Error ? err.message : (config.errorMessage ?? '操作失败'))
      return false
    } finally {
      acting.value = false
    }
  }

  return { acting, run }
}
