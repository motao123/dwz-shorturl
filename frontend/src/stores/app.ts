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

  const sidebarWidth = computed(() => (sidebarCollapsed.value ? 64 : 224))

  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
    localStorage.setItem(COLLAPSE_KEY, sidebarCollapsed.value ? '1' : '0')
  }

  function setBreadcrumbs(items: BreadcrumbItem[]) {
    breadcrumbs.value = items
  }

  return { sidebarCollapsed, sidebarWidth, breadcrumbs, toggleSidebar, setBreadcrumbs }
})
