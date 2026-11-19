import { defineStore } from 'pinia'

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
function write(key: string, v: string) {
  try {
    localStorage.setItem(key, v)
  } catch {
    /* ignore */
  }
}

export const useUiStore = defineStore('ui', {
  state: () => ({
    dark: typeof document !== 'undefined' && document.documentElement.classList.contains('app-dark'),
    sidebarCollapsed: read('qtf.sidebar') === 'collapsed',
    mobileNav: false,
  }),
  actions: {
    toggleDark() {
      this.dark = !this.dark
      document.documentElement.classList.toggle('app-dark', this.dark)
      write('qtf.theme', this.dark ? 'dark' : 'light')
    },
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
      write('qtf.sidebar', this.sidebarCollapsed ? 'collapsed' : 'expanded')
    },
  },
})

export { read as readPref, write as writePref }
