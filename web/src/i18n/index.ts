// vue-i18n instance and the app's active language (see select.ts for the rules).
import { createI18n } from 'vue-i18n'
import { setFormatLocale } from '@/lib/format'
import en from './locales/en'
import sv from './locales/sv'
import guideEn from './locales/guide.en'
import guideSv from './locales/guide.sv'
import { PRIMEVUE_LOCALES, type PrimeVueLocale } from './primevue'
import { chooseLanguage, DEFAULT_LANG, intlLocale, navigatorLanguages, normalizeLang, readStoredLang, writeStoredLang, type Lang } from './select'

export type { Lang } from './select'
export { LANGUAGES, LANGUAGE_NAMES } from './select'

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: DEFAULT_LANG as Lang,
  fallbackLocale: 'en',
  // user guide and intro tour texts (own files; titles.help is merged into titles)
  messages: {
    en: { ...en, ...guideEn, titles: { ...en.titles, ...guideEn.titles } },
    sv: { ...sv, ...guideSv, titles: { ...sv.titles, ...guideSv.titles } },
  },
  missingWarn: false,
  fallbackWarn: false,
})

/** Translate outside components (stores, helpers); reactive to the active language. */
export const t = i18n.global.t
/** Whether a key exists in the active language. */
export const te = i18n.global.te

let primeConfig: { locale?: unknown } | null = null

/** Current language. */
export function currentLang(): Lang {
  return normalizeLang(i18n.global.locale.value) ?? DEFAULT_LANG
}

/** Switch the UI language now: messages, <html lang>, Intl formatting and PrimeVue texts. */
export function setLanguage(lang: Lang): void {
  i18n.global.locale.value = lang
  setFormatLocale(intlLocale(lang, navigatorLanguages()))
  if (typeof document !== 'undefined') document.documentElement.lang = lang
  applyPrimeVue()
}

function applyPrimeVue() {
  if (!primeConfig) return
  const l: PrimeVueLocale = PRIMEVUE_LOCALES[currentLang()]
  primeConfig.locale = { ...l, aria: { ...l.aria } }
}

/** Let PrimeVue follow the language (call with usePrimeVue().config from App.vue). */
export function bindPrimeVue(config: { locale?: unknown }): void {
  primeConfig = config
  applyPrimeVue()
}

/** Language for a user preference (null/undefined = not set): preference, device choice, browser, English. */
export function resolveLanguage(preference?: unknown): Lang {
  return chooseLanguage({ preference, stored: readStoredLang(), languages: navigatorLanguages() })
}

/** Apply the signed-in user's preference (or the fallbacks when it is not set). */
export function applyPreference(preference?: unknown): Lang {
  const lang = resolveLanguage(preference)
  setLanguage(lang)
  return lang
}

/** A choice made on this device (login page, top bar): remember it locally and switch now. */
export function chooseLocal(lang: Lang | null): Lang {
  writeStoredLang(lang)
  const next = lang ?? resolveLanguage()
  setLanguage(next)
  return next
}

setLanguage(resolveLanguage())
