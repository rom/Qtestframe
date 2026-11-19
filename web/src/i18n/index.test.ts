import { afterEach, describe, expect, it } from 'vitest'
import { getFormatLocale, relativeTime } from '@/lib/format'
import { applyPreference, bindPrimeVue, chooseLocal, currentLang, i18n, resolveLanguage, setLanguage, t } from './index'
import { STORAGE_KEY } from './select'

function setBrowserLanguages(langs: string[]) {
  Object.defineProperty(window.navigator, 'languages', { value: langs, configurable: true })
}

afterEach(() => {
  localStorage.clear()
  setBrowserLanguages(['en-US', 'en'])
  setLanguage('en')
})

describe('i18n instance', () => {
  it('uses the Composition API mode', () => {
    expect(i18n.mode).toBe('composition')
    expect(i18n.global.fallbackLocale.value).toBe('en')
  })
  it('translates and pluralises in English', () => {
    setLanguage('en')
    expect(t('nav.requirements')).toBe('Requirements')
    expect(t('counts.requirements', 0)).toBe('no requirements')
    expect(t('counts.requirements', 1)).toBe('1 requirement')
    expect(t('counts.requirements', 3)).toBe('3 requirements')
    expect(t('common.selected', { count: 2 })).toBe('2 selected')
  })
  it('translates and pluralises in Swedish', () => {
    setLanguage('sv')
    expect(t('nav.requirements')).toBe('Krav')
    expect(t('common.save')).toBe('Spara')
    expect(t('counts.requirements', 1)).toBe('1 krav')
    expect(t('counts.requirements', 3)).toBe('3 krav')
    expect(t('counts.tests', 2)).toBe('2 testfall')
    expect(t('counts.runs', 1)).toBe('1 körning')
    expect(t('counts.runs', 4)).toBe('4 körningar')
    expect(t('projects.confirmDelete', { key: 'PAY', name: 'Betalning' })).toContain('PAY ”Betalning”')
  })
})

describe('setLanguage', () => {
  it('sets <html lang>, the Intl locale and the PrimeVue texts', () => {
    const config: { locale?: unknown } = {}
    bindPrimeVue(config)
    setLanguage('sv')
    expect(document.documentElement.lang).toBe('sv')
    expect(getFormatLocale()).toBe('sv-SE')
    expect(currentLang()).toBe('sv')
    const sv = config.locale as { dayNames: string[]; accept: string; firstDayOfWeek: number; aria: { close: string } }
    expect(sv.dayNames[1]).toBe('måndag')
    expect(sv.accept).toBe('Ja')
    expect(sv.firstDayOfWeek).toBe(1)
    expect(sv.aria.close).toBe('Stäng')
    expect(relativeTime(new Date(Date.now() - 5 * 60_000))).toBe('för 5 minuter sedan')
    setLanguage('en')
    expect(document.documentElement.lang).toBe('en')
    expect(getFormatLocale()).toBe('en-US')
    expect((config.locale as { dayNames: string[] }).dayNames[1]).toBe('Monday')
  })
})

describe('language selection', () => {
  it('prefers the user preference', () => {
    localStorage.setItem(STORAGE_KEY, 'en')
    expect(applyPreference('sv')).toBe('sv')
    expect(i18n.global.locale.value).toBe('sv')
  })
  it('falls back to the device choice, then the browser, then English', () => {
    localStorage.setItem(STORAGE_KEY, 'sv')
    expect(applyPreference(null)).toBe('sv')
    localStorage.clear()
    setBrowserLanguages(['sv-FI', 'en'])
    expect(resolveLanguage(undefined)).toBe('sv')
    setBrowserLanguages(['de-DE'])
    expect(resolveLanguage('')).toBe('en')
  })
  it('remembers a local choice and can forget it', () => {
    expect(chooseLocal('sv')).toBe('sv')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('sv')
    expect(currentLang()).toBe('sv')
    expect(chooseLocal(null)).toBe('en')
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull()
  })
})
