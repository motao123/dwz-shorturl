import { ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import dayjs from 'dayjs'

export interface UseExportCsvOptions {
  /** 拉取 CSV 二进制内容 */
  fetcher: () => Promise<Blob>
  /** 文件名前缀，默认 `export` */
  filename?: string | (() => string)
  /** 是否追加 `YYYYMMDD-HHmm` 时间戳（默认 true，避免同名覆盖） */
  timestamp?: boolean
  successMessage?: string
}

/**
 * CSV 导出：请求 → 触发下载 → 释放 blob URL → 结果提示。
 *
 * 释放前留 1s 缓冲：部分浏览器立即 revoke 会中断下载。
 */
export function useExportCsv(options: UseExportCsvOptions) {
  const exporting = ref(false)

  /** 覆盖页面侧兜底，如「登录状态已失效，请重新登录后再导出」 */
  const errorMessage = ref('导出失败')

  function resolveFilename(): string {
    const base = typeof options.filename === 'function' ? options.filename() : (options.filename ?? 'export')
    if (options.timestamp === false) return `${base}.csv`
    return `${base}-${dayjs().format('YYYYMMDD-HHmm')}.csv`
  }

  async function exportCsv(): Promise<boolean> {
    exporting.value = true
    try {
      const blob = await options.fetcher()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = resolveFilename()
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
      ElMessage.success(options.successMessage ?? 'CSV 导出成功')
      return true
    } catch (err) {
      ElMessage.error(err instanceof Error && err.message ? err.message : errorMessage.value)
      return false
    } finally {
      exporting.value = false
    }
  }

  return { exporting, errorMessage, exportCsv }
}
