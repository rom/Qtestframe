import { defineStore } from 'pinia'
import { computed, watch } from 'vue'
import { api } from '@/lib/api'
import { setDatePrefs } from '@/lib/format'
import {
  createLinkedTestDefault,
  LOCAL_DECOR_KEY,
  LOCAL_SCHEME_KEY,
  localThemeEntries,
  normalizePrefs,
  pageSize,
  pickLocalTheme,
  readLastProject,
  resolveTheme,
  rowsOptions,
  writeLastProject,
} from '@/lib/prefs'
import type { ThemeId } from '@/themes/registry'
import type { Me, Preferences } from '@/lib/types'
import { useAuthStore } from './auth'
import { useUiStore, readPref, writePref, removePref } from './ui'

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
  // --- obligation keyword highlighting (on unless turned off) ---
  const highlightObligations = computed(() => !prefs.value.hide_obligation_highlight)

  function createLinkedTest(serverDefault: boolean | undefined): boolean {
    return createLinkedTestDefault(prefs.value, serverDefault)
  }

  /** Rows option list for tables with their own default (e.g. 50). */
  function rowsFor(fallback: number): number {
    return pageSize(prefs.value, fallback)
  }

  /** The theme preference ('' = not set: this device's last theme or the OS). */
  const themePref = computed<ThemeId | ''>(() => (prefs.value.theme as ThemeId | undefined) ?? '')
  /** The painted theme (never "system"). */
  const theme = computed(() => ui.theme)

  function localTheme() {
    return pickLocalTheme(readPref(LOCAL_DECOR_KEY), readPref(LOCAL_SCHEME_KEY))
  }

  /** Remember a theme on this device (theme-init.js reads the scheme before first paint). */
  function rememberLocal(id: ThemeId) {
    const { scheme, decor } = localThemeEntries(id)
    if (scheme) writePref(LOCAL_SCHEME_KEY, scheme)
    else removePref(LOCAL_SCHEME_KEY)
    if (decor) writePref(LOCAL_DECOR_KEY, decor)
    else removePref(LOCAL_DECOR_KEY)
  }

  function applyTheme() {
    const p = prefs.value.theme
    ui.setTheme(resolveTheme(p, localTheme(), systemDark()))
    // keep theme-init.js (first paint) in line with the preference
    if (p) rememberLocal(p as ThemeId)
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

  /**
   * Choose a theme (any preference value, "system" included): paint it now,
   * remember it on this device and store it as preference (PATCH /auth/me).
   */
  async function setTheme(id: ThemeId) {
    ui.setTheme(resolveTheme(id, null, systemDark()))
    rememberLocal(id)
    if (!auth.me) return
    try {
      await save({ theme: id })
    } catch (e) {
      console.warn('saving theme preference failed', e)
    }
  }

  /** Top-bar toggle: light ↔ dark (from a decorative theme: to the opposite plain scheme). */
  async function toggleTheme() {
    await setTheme(ui.dark ? 'light' : 'dark')
  }

  function rememberProject(projectId: string) {
    writeLastProject(auth.tenant?.id, projectId)
  }
  function lastProject(): string | null {
    return readLastProject(auth.tenant?.id)
  }

  return { prefs, rows, rowsPerPage, hideAnalysis, highlightObligations, themePref, theme, createLinkedTest, rowsFor, apply, start, save, setTheme, toggleTheme, rememberProject, lastProject }
})
