import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export interface BreadcrumbItem {
  title: string
  path?: string
}

const COLLAPSE_KEY = 'dwz_sidebar_collapsed'

export const useAppStore = defineStore('app', () => {
  const sidebarCollapsed = ref(localStorage.getItem(COLLAPSE_KEY) === '1')
  const breadcrumbs = ref<BreadcrumbItem[]>([])

  // true once the user manually toggled the sidebar; used so the responsive
  // auto-collapse does not fight the user's explicit choice.
  const userCollapsed = ref(false)

  const sidebarWidth = computed(() => (sidebarCollapsed.value ? 64 : 224))

  function toggleSidebar() {
    userCollapsed.value = true
    sidebarCollapsed.value = !sidebarCollapsed.value
    localStorage.setItem(COLLAPSE_KEY, sidebarCollapsed.value ? '1' : '0')
  }

  function setSidebarCollapsed(v: boolean) {
    sidebarCollapsed.value = v
    localStorage.setItem(COLLAPSE_KEY, v ? '1' : '0')
  }

  function setBreadcrumbs(items: BreadcrumbItem[]) {
    breadcrumbs.value = items
  }

  return { sidebarCollapsed, sidebarWidth, breadcrumbs, userCollapsed, toggleSidebar, setSidebarCollapsed, setBreadcrumbs }
})
