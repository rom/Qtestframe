// Language selection: pure decisions, no Vue or i18n instance involved.

export const LANGUAGES = ['en', 'sv'] as const
export type Lang = (typeof LANGUAGES)[number]
export const DEFAULT_LANG: Lang = 'en'

/** localStorage key for the language chosen on this device (login page, top bar). */
export const STORAGE_KEY = 'qtf.lang'

/** Each language's name in its own language (for the language switchers). */
export const LANGUAGE_NAMES: Record<Lang, string> = { en: 'English', sv: 'Svenska' }

/** BCP 47 tags used for Intl formatting. */
export const INTL_LOCALES: Record<Lang, string> = { en: 'en-GB', sv: 'sv-SE' }

/** "sv", "sv-SE", "SV_fi" → "sv"; "en-US" → "en"; anything else → null. */
export function normalizeLang(v: unknown): Lang | null {
  if (typeof v !== 'string') return null
  const base = v.trim().toLowerCase().split(/[-_]/)[0]
  return (LANGUAGES as readonly string[]).includes(base) ? (base as Lang) : null
}

/** First supported language in the browser's preference list (any sv* → sv). */
export function browserLang(languages: readonly (string | null | undefined)[] | null | undefined): Lang | null {
  for (const l of languages ?? []) {
    const n = normalizeLang(l)
    if (n) return n
  }
  return null
}

export interface LanguageSources {
  /** user.preferences.language from /auth/me */
  preference?: unknown
  /** choice stored on this device (qtf.lang) */
  stored?: unknown
  /** navigator.languages */
  languages?: readonly (string | null | undefined)[] | null
}

/** Preference, then this device's choice, then the browser, then English. */
export function chooseLanguage(src: LanguageSources = {}): Lang {
  return normalizeLang(src.preference) ?? normalizeLang(src.stored) ?? browserLang(src.languages) ?? DEFAULT_LANG
}

type KV = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

function storage(): KV | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

export function readStoredLang(store: KV | null = storage()): Lang | null {
  try {
    return normalizeLang(store?.getItem(STORAGE_KEY))
  } catch {
    return null
  }
}

/** Remember (or with null forget) this device's language choice. */
export function writeStoredLang(lang: Lang | null, store: KV | null = storage()): void {
  try {
    if (lang) store?.setItem(STORAGE_KEY, lang)
    else store?.removeItem(STORAGE_KEY)
  } catch {
    /* storage unavailable */
  }
}

/** navigator.languages (or navigator.language) without throwing. */
export function navigatorLanguages(nav: { languages?: readonly string[]; language?: string } | null | undefined = typeof navigator === 'undefined' ? null : navigator): string[] {
  if (!nav) return []
  if (Array.isArray(nav.languages) && nav.languages.length) return [...nav.languages]
  return nav.language ? [nav.language] : []
}

/**
 * Intl locale for a language: the browser's own regional variant when it
 * matches (e.g. en-US), else the default region (en-GB, sv-SE).
 */
export function intlLocale(lang: Lang, languages: readonly (string | null | undefined)[] | null | undefined = []): string {
  for (const l of languages ?? []) {
    if (l && normalizeLang(l) === lang && /^[a-z]{2,3}[-_][a-z0-9]{2,8}$/i.test(l)) return l.replace('_', '-')
  }
  return INTL_LOCALES[lang]
}
