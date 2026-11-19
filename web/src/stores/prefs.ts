import { defineStore } from 'pinia'
import { computed, watch } from 'vue'
import { api } from '@/lib/api'
import { setDatePrefs } from '@/lib/format'
import { createLinkedTestDefault, normalizePrefs, pageSize, readLastProject, resolveDark, rowsOptions, writeLastProject } from '@/lib/prefs'
import type { Me, Preferences } from '@/lib/types'
import { useAuthStore } from './auth'
import { useUiStore, readPref, writePref } from './ui'

function systemDark(): boolean {
  try {
    return typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-color-scheme: dark)').matches
  } catch {
    return false
  }
}

/**
 * The signed-in user's preferences (Me.user.preferences) and their effects
 * on the app: theme, compact tables, date formatting, page sizes.
 */
export const usePrefsStore = defineStore('prefs', () => {
  const auth = useAuthStore()
  const ui = useUiStore()

  const prefs = computed<Preferences>(() => normalizePrefs(auth.me?.user?.preferences))
  const rows = computed(() => pageSize(prefs.value))
  const rowsPerPage = computed(() => rowsOptions(rows.value))
  const hideAnalysis = computed(() => !!prefs.value.hide_analysis)

  function createLinkedTest(serverDefault: boolean | undefined): boolean {
    return createLinkedTestDefault(prefs.value, serverDefault)
  }

  /** Rows option list for tables with their own default (e.g. 50). */
  function rowsFor(fallback: number): number {
    return pageSize(prefs.value, fallback)
  }

  function applyTheme() {
    const p = prefs.value.theme
    const dark = resolveDark(p, readPref('qtf.theme'), systemDark())
    ui.setDark(dark)
    // keep theme-init.js (first paint) in line with the preference
    if (p === 'light' || p === 'dark') writePref('qtf.theme', p)
    else if (p === 'system') removeLocalTheme()
  }

  function apply() {
    applyTheme()
    if (typeof document !== 'undefined') document.documentElement.classList.toggle('compact-tables', !!prefs.value.compact_tables)
    setDatePrefs(prefs.value)
  }

  let started = false
  /** Start applying preferences whenever they change (call once at app start). */
  function start() {
    if (started) return
    started = true
    watch(prefs, apply, { immediate: true, deep: true })
    try {
      window.matchMedia?.('(prefers-color-scheme: dark)').addEventListener?.('change', () => applyTheme())
    } catch {
      /* old browsers */
    }
  }

  /** PATCH /auth/me with some preference keys (null resets a key). */
  async function save(patch: Record<string, unknown>): Promise<Me> {
    const me = await api.patch<Me>('/auth/me', { preferences: patch })
    if (me?.user) auth.set(me)
    return me
  }

  /** Top-bar toggle: switch now, remember locally and store as preference. */
  async function toggleTheme() {
    const dark = !ui.dark
    ui.setDark(dark)
    writePref('qtf.theme', dark ? 'dark' : 'light')
    if (!auth.me) return
    try {
      await save({ theme: dark ? 'dark' : 'light' })
    } catch (e) {
      console.warn('saving theme preference failed', e)
    }
  }

  function rememberProject(projectId: string) {
    writeLastProject(auth.tenant?.id, projectId)
  }
  function lastProject(): string | null {
    return readLastProject(auth.tenant?.id)
  }

  return { prefs, rows, rowsPerPage, hideAnalysis, createLinkedTest, rowsFor, apply, start, save, toggleTheme, rememberProject, lastProject }
})

function removeLocalTheme() {
  try {
    localStorage.removeItem('qtf.theme')
  } catch {
    /* ignore */
  }
}
