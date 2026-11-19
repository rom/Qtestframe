import { defineStore } from 'pinia'
import { applyTheme } from '@/themes/apply'
import { THEME_DEFS, isThemeId, type ConcreteThemeId } from '@/themes/registry'

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

function initialTheme(): ConcreteThemeId {
  if (typeof document === 'undefined') return 'light'
  const root = document.documentElement
  const t = root.getAttribute('data-theme')
  if (isThemeId(t) && t !== 'system') return t
  return root.classList.contains('app-dark') ? 'dark' : 'light'
}

export const useUiStore = defineStore('ui', {
  state: () => ({
    dark: typeof document !== 'undefined' && document.documentElement.classList.contains('app-dark'),
    /** the painted theme (never "system"); see setTheme */
    theme: initialTheme(),
    sidebarCollapsed: read('qtf.sidebar') === 'collapsed',
    mobileNav: false,
  }),
  getters: {
    /** true for spring, navy, … (not light/dark) */
    decorative: (s) => THEME_DEFS[s.theme].decorative,
  },
  actions: {
    toggleDark() {
      this.setDark(!this.dark)
      write('qtf.theme', this.dark ? 'dark' : 'light')
      remove('qtf.decor')
    },
    /** Apply plain light or dark without persisting it (leaves a decorative theme). */
    setDark(dark: boolean) {
      this.setTheme(dark ? 'dark' : 'light')
    },
    /** Paint a theme (light, dark or decorative) without persisting it. */
    setTheme(id: ConcreteThemeId) {
      applyTheme(id)
      this.theme = id
      this.dark = THEME_DEFS[id].scheme === 'dark'
    },
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
      write('qtf.sidebar', this.sidebarCollapsed ? 'collapsed' : 'expanded')
    },
  },
})

function remove(key: string) {
  try {
    localStorage.removeItem(key)
  } catch {
    /* ignore */
  }
}

export { read as readPref, write as writePref, remove as removePref }
