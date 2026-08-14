import { defineStore } from 'pinia'
import { ref } from 'vue'

const KEY = 'dwz_theme'

export function applyDark(dark: boolean) {
  document.documentElement.classList.toggle('dark', dark)
}

export function initTheme() {
  const saved = localStorage.getItem(KEY)
  const dark = saved === 'dark' || (!saved && window.matchMedia?.('(prefers-color-scheme: dark)').matches)
  applyDark(dark)
}

export const useThemeStore = defineStore('theme', () => {
  const dark = ref(document.documentElement.classList.contains('dark'))

  function toggle() {
    dark.value = !dark.value
    applyDark(dark.value)
    localStorage.setItem(KEY, dark.value ? 'dark' : 'light')
  }

  return { dark, toggle }
})