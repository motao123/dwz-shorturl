import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue'

/**
 * 视口宽度断点（与 styles/index.scss 中的 @media 保持一致）。
 * 管理台侧栏折叠 / 表格横向滚动用的是 992px，这里沿用同一阈值。
 */
export const BREAKPOINT_TABLET = 992
export const BREAKPOINT_MOBILE = 640

function matchMaxWidth(px: number): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false
  return window.matchMedia(`(max-width: ${px}px)`).matches
}

/**
 * 响应式断点状态。返回的 ref 会随窗口尺寸变化实时更新，
 * 用于按屏宽切换分页器布局、表格列等「必须靠 JS 决定」的渲染结构。
 *
 * 纯样式切换（隐藏/换行）仍应交给 CSS @media，避免额外的 JS 依赖。
 */
export function useResponsive() {
  const isTablet = ref(matchMaxWidth(BREAKPOINT_TABLET))
  const isMobile = ref(matchMaxWidth(BREAKPOINT_MOBILE))

  const listeners: Array<() => void> = []

  onMounted(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return
    const tabletMq = window.matchMedia(`(max-width: ${BREAKPOINT_TABLET}px)`)
    const mobileMq = window.matchMedia(`(max-width: ${BREAKPOINT_MOBILE}px)`)
    const onTablet = (e: MediaQueryListEvent) => (isTablet.value = e.matches)
    const onMobile = (e: MediaQueryListEvent) => (isMobile.value = e.matches)
    tabletMq.addEventListener('change', onTablet)
    mobileMq.addEventListener('change', onMobile)
    listeners.push(() => {
      tabletMq.removeEventListener('change', onTablet)
      mobileMq.removeEventListener('change', onMobile)
    })
  })

  onBeforeUnmount(() => listeners.forEach((off) => off()))

  return { isTablet, isMobile } as { isTablet: Ref<boolean>; isMobile: Ref<boolean> }
}
