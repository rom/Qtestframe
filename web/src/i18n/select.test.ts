import { describe, expect, it } from 'vitest'
import { browserLang, chooseLanguage, intlLocale, LANGUAGE_NAMES, navigatorLanguages, normalizeLang, readStoredLang, STORAGE_KEY, writeStoredLang } from './select'

function memStore(init: Record<string, string> = {}) {
  const m = new Map(Object.entries(init))
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
    map: m,
  }
}
const broken = {
  getItem: () => {
    throw new Error('denied')
  },
  setItem: () => {
    throw new Error('denied')
  },
  removeItem: () => {
    throw new Error('denied')
  },
}

describe('normalizeLang', () => {
  it('maps tags to supported languages', () => {
    expect(normalizeLang('sv')).toBe('sv')
    expect(normalizeLang('sv-SE')).toBe('sv')
    expect(normalizeLang('SV_fi')).toBe('sv')
    expect(normalizeLang(' en-US ')).toBe('en')
    expect(normalizeLang('de')).toBeNull()
    expect(normalizeLang('')).toBeNull()
    expect(normalizeLang(null)).toBeNull()
    expect(normalizeLang(42)).toBeNull()
  })
})

describe('browserLang', () => {
  it('takes the first supported language; any sv* is Swedish', () => {
    expect(browserLang(['sv-FI', 'en'])).toBe('sv')
    expect(browserLang(['de-DE', 'sv'])).toBe('sv')
    expect(browserLang(['de-DE', 'en-GB', 'sv'])).toBe('en')
    expect(browserLang(['fr', 'de'])).toBeNull()
    expect(browserLang([null, undefined, ''])).toBeNull()
    expect(browserLang(undefined)).toBeNull()
  })
})

describe('chooseLanguage', () => {
  it('prefers the user preference', () => {
    expect(chooseLanguage({ preference: 'sv', stored: 'en', languages: ['en'] })).toBe('sv')
    expect(chooseLanguage({ preference: 'en', stored: 'sv', languages: ['sv'] })).toBe('en')
  })
  it('then the device choice', () => {
    expect(chooseLanguage({ preference: null, stored: 'sv', languages: ['en'] })).toBe('sv')
    expect(chooseLanguage({ preference: 'xx', stored: 'en', languages: ['sv'] })).toBe('en')
  })
  it('then the browser, then English', () => {
    expect(chooseLanguage({ languages: ['sv-SE'] })).toBe('sv')
    expect(chooseLanguage({ languages: ['ja'] })).toBe('en')
    expect(chooseLanguage()).toBe('en')
  })
})

describe('stored language', () => {
  it('reads and writes qtf.lang', () => {
    const s = memStore()
    expect(readStoredLang(s)).toBeNull()
    writeStoredLang('sv', s)
    expect(s.map.get(STORAGE_KEY)).toBe('sv')
    expect(readStoredLang(s)).toBe('sv')
    writeStoredLang(null, s)
    expect(s.map.has(STORAGE_KEY)).toBe(false)
    expect(readStoredLang(memStore({ [STORAGE_KEY]: 'klingon' }))).toBeNull()
  })
  it('survives unavailable storage', () => {
    expect(readStoredLang(broken)).toBeNull()
    expect(() => writeStoredLang('sv', broken)).not.toThrow()
    expect(readStoredLang(null)).toBeNull()
    expect(() => writeStoredLang('en', null)).not.toThrow()
  })
  it('uses localStorage by default', () => {
    writeStoredLang('sv')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('sv')
    expect(readStoredLang()).toBe('sv')
    writeStoredLang(null)
    expect(readStoredLang()).toBeNull()
  })
})

describe('navigator languages', () => {
  it('uses languages, then language', () => {
    expect(navigatorLanguages({ languages: ['sv-SE', 'en'] })).toEqual(['sv-SE', 'en'])
    expect(navigatorLanguages({ languages: [], language: 'sv' })).toEqual(['sv'])
    expect(navigatorLanguages({})).toEqual([])
    expect(navigatorLanguages(null)).toEqual([])
    expect(Array.isArray(navigatorLanguages())).toBe(true)
  })
})

describe('intlLocale', () => {
  it('keeps the browser region of the same language', () => {
    expect(intlLocale('en', ['en-US'])).toBe('en-US')
    expect(intlLocale('sv', ['sv_FI'])).toBe('sv-FI')
    expect(intlLocale('sv', ['en-US', 'sv'])).toBe('sv-SE')
    expect(intlLocale('en', ['sv-SE'])).toBe('en-GB')
    expect(intlLocale('en', null)).toBe('en-GB')
    expect(intlLocale('sv')).toBe('sv-SE')
  })
  it('names languages in their own language', () => {
    expect(LANGUAGE_NAMES).toEqual({ en: 'English', sv: 'Svenska' })
  })
})
