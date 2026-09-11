import { computed, type ComputedRef } from 'vue'
import { useThemeStore } from '@/stores/theme'

/**
 * ECharts 主题 token。
 *
 * 图表 option 里此前把坐标轴/分割线/文字颜色写死为浅色值，切到暗色模式后
 * 这些颜色落在深色底上几乎不可见（趋势图看起来是空白）。这里统一从 CSS
 * 变量读取，颜色定义仍在 styles/index.scss 的 :root / html.dark 中维护。
 */
export interface ChartTheme {
  axis: string
  grid: string
  label: string
  primary: string
  primary2: string
  accent: string
  tooltipBg: string
  tooltipFg: string
  fontMono: string
}

function readVar(name: string, fallback: string): string {
  if (typeof window === 'undefined' || !window.getComputedStyle) return fallback
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

export function useChartTheme(): ComputedRef<ChartTheme> {
  const themeStore = useThemeStore()

  return computed<ChartTheme>(() => {
    // 依赖 themeStore.dark，切换主题时重新计算，图表 option 随之重绘。
    void themeStore.dark
    return {
      axis: readVar('--dwz-chart-axis', '#6b7f86'),
      grid: readVar('--dwz-chart-grid', '#e8eef0'),
      label: readVar('--dwz-chart-label', '#1f3238'),
      primary: readVar('--dwz-chart-primary', '#0e6e75'),
      primary2: readVar('--dwz-chart-primary-2', '#2fa3a8'),
      accent: readVar('--dwz-chart-accent', '#f5a623'),
      tooltipBg: readVar('--dwz-chart-tooltip-bg', 'rgba(12, 42, 48, 0.92)'),
      tooltipFg: readVar('--dwz-chart-tooltip-fg', '#e8f4f2'),
      fontMono: readVar('--dwz-font-mono', 'JetBrains Mono, monospace')
    }
  })
}
