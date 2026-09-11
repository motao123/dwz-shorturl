import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('element-plus/es/components/message/index', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() }
}))
vi.mock('element-plus/es/components/message-box/index', () => ({
  ElMessageBox: { confirm: vi.fn() }
}))

import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { notifyBatchSyncResult, notifySyncResult, useBatchAction } from '../useBatchAction'

interface Row {
  id: number
  uid: string
  [key: string]: unknown
}

function rows(...ids: number[]): Row[] {
  return ids.map((id) => ({ id, uid: `u${id}` }))
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useBatchAction', () => {
  it('未勾选时提示并不执行请求', async () => {
    const batch = useBatchAction<Row>()
    const run = vi.fn()
    const ok = await batch.execute(run)
    expect(ok).toBe(false)
    expect(run).not.toHaveBeenCalled()
    expect(ElMessage.warning).toHaveBeenCalledWith('请先勾选要操作的数据')
  })

  it('selectedIds 按 rowKey 提取，过滤非法 id', () => {
    const batch = useBatchAction<Row>()
    batch.setSelected([...rows(1, 2), { id: Number.NaN, uid: 'x' }])
    expect(batch.selectedIds.value).toEqual([1, 2])
    expect(batch.selectedCount.value).toBe(3)
    expect(batch.hasSelection.value).toBe(true)
  })

  it('确认后执行，成功后清空勾选', async () => {
    vi.mocked(ElMessageBox.confirm).mockResolvedValue('confirm' as never)
    const batch = useBatchAction<Row>()
    batch.setSelected(rows(1, 2))
    const run = vi.fn().mockResolvedValue({ ok: true })
    const ok = await batch.execute(run, { confirm: { message: () => '确认删除？' } })
    expect(ok).toBe(true)
    expect(run).toHaveBeenCalledWith([1, 2])
    expect(batch.hasSelection.value).toBe(false)
    expect(batch.executing.value).toBe(false)
    expect(vi.mocked(ElMessageBox.confirm).mock.calls[0][0]).toBe('确认删除？')
    expect(ElMessage.success).toHaveBeenCalledWith('已处理 2 条')
  })

  it('用户取消确认时不执行请求', async () => {
    vi.mocked(ElMessageBox.confirm).mockRejectedValue(new Error('cancel'))
    const batch = useBatchAction<Row>()
    batch.setSelected(rows(1))
    const run = vi.fn()
    const ok = await batch.execute(run, { confirm: { message: '确认？' } })
    expect(ok).toBe(false)
    expect(run).not.toHaveBeenCalled()
    expect(batch.hasSelection.value).toBe(true)
  })

  it('公共库同步失败：走统一契约提示并列出失败 UID', async () => {
    const batch = useBatchAction<Row>()
    batch.setSelected(rows(1, 2, 3))
    const ok = await batch.execute(
      async () => ({ public_sync_failed: true, sync_failed_uids: ['u1', 'u3'] }),
      { confirm: null }
    )
    expect(ok).toBe(true)
    expect(ElMessage.warning).toHaveBeenCalledTimes(1)
    const message = vi.mocked(ElMessage.warning).mock.calls[0][0] as { message: string; duration: number }
    expect(message.message).toContain('wjoy_log')
    expect(message.message).toContain('u1、u3')
    expect(message.duration).toBe(8000)
  })

  it('请求异常时提示错误且不清空勾选', async () => {
    const batch = useBatchAction<Row>()
    batch.setSelected(rows(1))
    const ok = await batch.execute(async () => {
      throw new Error('后端拒绝')
    })
    expect(ok).toBe(false)
    expect(ElMessage.error).toHaveBeenCalledWith('后端拒绝')
    expect(batch.hasSelection.value).toBe(true)
  })

  it('notifySync=false 时使用自定义成功文案，不消费同步契约', async () => {
    const batch = useBatchAction<Row>()
    batch.setSelected(rows(1))
    const ok = await batch.execute(async () => ({ updated: 1 }), {
      notifySync: false,
      successMessage: (n) => `已更新 ${n} 条`
    })
    expect(ok).toBe(true)
    expect(ElMessage.success).toHaveBeenCalledWith('已更新 1 条')
  })
})

describe('同步失败契约提示', () => {
  it('notifySyncResult：无同步失败 → success', () => {
    notifySyncResult({}, '删除成功')
    expect(ElMessage.success).toHaveBeenCalledWith('删除成功')
  })

  it('notifySyncResult：同步失败 → warning 且带 6s 时长', () => {
    notifySyncResult({ public_sync_failed: true, warning: '补充说明' }, '删除成功')
    const arg = vi.mocked(ElMessage.warning).mock.calls[0][0] as { message: string; duration: number }
    expect(arg.message).toContain('补充说明')
    expect(arg.duration).toBe(6000)
  })

  it('notifySyncResult：空响应视为成功', () => {
    notifySyncResult(null, '删除成功')
    expect(ElMessage.success).toHaveBeenCalledWith('删除成功')
  })

  it('notifyBatchSyncResult：无同步失败 → success 带条数', () => {
    notifyBatchSyncResult({}, 3)
    expect(ElMessage.success).toHaveBeenCalledWith('已处理 3 条')
  })

  it('支持自定义文案', () => {
    notifyBatchSyncResult(
      { public_sync_failed: true },
      1,
      { batch: '短链删除后公共库同步失败。' }
    )
    const arg = vi.mocked(ElMessage.warning).mock.calls[0][0] as { message: string }
    expect(arg.message).toBe('短链删除后公共库同步失败。')
  })
})
