// Paint a theme on the document: .app-dark (PrimeVue's darkModeSelector),
// data-theme (the --qtf-* variables in styles.css), color-scheme,
// <meta name="theme-color"> and, when it changes, the PrimeVue preset.
import { usePreset } from '@primevue/themes'
import { LOCAL_DECOR_KEY, LOCAL_SCHEME_KEY, pickLocalTheme, resolveTheme } from '@/lib/prefs'
import { presetFor, presetKey, type PresetKey } from './presets'
import { THEME_DEFS, type ConcreteThemeId } from './registry'

let currentKey: PresetKey | null = null

export interface ApplyEnv {
  doc?: Document
  /** replaces the PrimeVue preset (default: usePreset from @primevue/themes) */
  setPreset?: (preset: unknown) => void
}

/** Apply a painted theme to the document. Switches the PrimeVue preset only when needed. */
export function applyTheme(id: ConcreteThemeId, env: ApplyEnv = {}): void {
  const doc = env.doc ?? (typeof document === 'undefined' ? undefined : document)
  const def = THEME_DEFS[id]
  if (doc) {
    const root = doc.documentElement
    const dark = def.scheme === 'dark'
    root.classList.toggle('app-dark', dark)
    root.setAttribute('data-theme', id)
    root.style.colorScheme = dark ? 'dark' : 'light'
    let meta = doc.head.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
    if (!meta) {
      meta = doc.createElement('meta')
      meta.name = 'theme-color'
      doc.head.appendChild(meta)
    }
    meta.content = def.colors!.topbarBg
  }
  const key = presetKey(id)
  if (currentKey !== null && key !== currentKey) {
    // usePreset replaces the whole preset and makes PrimeVue regenerate its CSS variables
    ;(env.setPreset ?? ((p: unknown) => usePreset(p as Record<string, unknown>)))(presetFor(id))
  }
  currentKey = key
}

/** The preset currently installed (null before bootTheme / the first applyTheme). */
export function currentPresetKey(): PresetKey | null {
  return currentKey
}

/** Forget the installed preset (tests). */
export function resetThemeState(): void {
  currentKey = null
}

type KV = Pick<Storage, 'getItem'>

function systemDark(): boolean {
  try {
    return typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-color-scheme: dark)').matches
  } catch {
    return false
  }
}

function localStore(): KV | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

/** This device's remembered theme (before the user's preferences are loaded). */
export function localTheme(store: KV | null = localStore(), dark = systemDark()): ConcreteThemeId {
  let decor: string | null = null
  let scheme: string | null = null
  try {
    decor = store?.getItem(LOCAL_DECOR_KEY) ?? null
    scheme = store?.getItem(LOCAL_SCHEME_KEY) ?? null
  } catch {
    /* storage unavailable */
  }
  return resolveTheme(null, pickLocalTheme(decor, scheme), dark)
}

/**
 * Call once in main.ts before app.use(PrimeVue): paints this device's last
 * theme and returns the preset to install, so there is no flash of the
 * default preset.
 */
export function bootTheme(env: ApplyEnv & { store?: KV | null; systemDark?: boolean } = {}): { id: ConcreteThemeId; preset: unknown } {
  const id = localTheme(env.store === undefined ? localStore() : env.store, env.systemDark ?? systemDark())
  currentKey = null
  applyTheme(id, env)
  return { id, preset: presetFor(id) }
}
