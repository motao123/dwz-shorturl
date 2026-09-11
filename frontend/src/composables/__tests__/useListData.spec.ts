import { describe, expect, it, vi } from 'vitest'

vi.mock('element-plus/es/components/message/index', () => ({
  ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() }
}))

import { ElMessage } from 'element-plus/es/components/message/index'
import { flushPromises } from './test-utils'
import { useListData } from '../useListData'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (err: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('useListData', () => {
  it('加载成功：写入列表与总数，关闭 loading', async () => {
    const data = useListData({ fetcher: async () => ({ list: [{ id: 1 }], total: 1, page: 1, per_page: 20 }) })
    await data.load()
    expect(data.rows.value).toEqual([{ id: 1 }])
    expect(data.total.value).toBe(1)
    expect(data.loading.value).toBe(false)
    expect(data.empty.value).toBe(false)
  })

  it('裸数组响应同样可归一化', async () => {
    const data = useListData({ fetcher: async () => [{ id: 1 }, { id: 2 }] })
    await data.load()
    expect(data.total.value).toBe(2)
  })

  it('失败：清空数据、给出错误提示，不抛出', async () => {
    const data = useListData({ fetcher: async () => Promise.reject(new Error('网络异常')) })
    await expect(data.load()).resolves.toBe(true)
    expect(data.rows.value).toEqual([])
    expect(data.total.value).toBe(0)
    expect(data.error.value?.message).toBe('网络异常')
    expect(ElMessage.error).toHaveBeenCalledWith('网络异常')
  })

  it('并发竞态：先发请求后返回时被丢弃，不覆盖新数据', async () => {
    const first = deferred<{ list: { id: number }[]; total: number; page: number; per_page: number }>()
    const second = deferred<{ list: { id: number }[]; total: number; page: number; per_page: number }>()
    const calls = [first.promise, second.promise]
    let index = 0
    const data = useListData({ fetcher: () => calls[index++] })

    const p1 = data.load()
    const p2 = data.load()
    // 新请求先返回
    second.resolve({ list: [{ id: 2 }], total: 1, page: 2, per_page: 20 })
    await flushPromises()
    expect(data.rows.value).toEqual([{ id: 2 }])
    expect(data.loading.value).toBe(false)

    // 旧请求后返回，必须被丢弃
    first.resolve({ list: [{ id: 1 }], total: 99, page: 1, per_page: 20 })
    await Promise.all([p1, p2])
    expect(data.rows.value).toEqual([{ id: 2 }])
    expect(data.total.value).toBe(1)
  })

  it('并发竞态：过期请求失败时不再弹错误提示', async () => {
    const first = deferred<never>()
    const second = deferred<{ list: never[]; total: number; page: number; per_page: number }>()
    const calls = [first.promise, second.promise]
    let index = 0
    const data = useListData({ fetcher: () => calls[index++] })

    const p1 = data.load()
    const p2 = data.load()
    second.resolve({ list: [], total: 0, page: 1, per_page: 20 })
    await flushPromises()
    vi.mocked(ElMessage.error).mockClear()

    first.reject(new Error('过期的失败'))
    await Promise.all([p1, p2])
    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  it('onLoaded 只在有效响应上执行', async () => {
    const onLoaded = vi.fn()
    const data = useListData({ fetcher: async () => ({ list: [{ id: 1 }], total: 1, page: 1, per_page: 20 }), onLoaded })
    await data.load()
    expect(onLoaded).toHaveBeenCalledWith([{ id: 1 }], 1)
  })

  it('刷新时默认静默（不切换 loading 骨架）', async () => {
    const data = useListData({ fetcher: async () => ({ list: [{ id: 1 }], total: 1, page: 1, per_page: 20 }) })
    await data.load()
    const pending = deferred<{ list: { id: number }[]; total: number; page: number; per_page: number }>()
    const data2 = useListData({ fetcher: async () => pending.promise })
    data2.hasLoaded.value = true
    const p = data2.load()
    expect(data2.loading.value).toBe(false)
    expect(data2.refreshing.value).toBe(true)
    pending.resolve({ list: [], total: 0, page: 1, per_page: 20 })
    await p
    expect(data2.refreshing.value).toBe(false)
  })
})
