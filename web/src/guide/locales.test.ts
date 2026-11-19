// The guide and tour texts: en and sv have the same keys and placeholders,
// nothing is empty, every tour step has a title and a body, and the texts
// reach the app's i18n instance.
import { describe, expect, it } from 'vitest'
import guideEn from '@/i18n/locales/guide.en'
import guideSv from '@/i18n/locales/guide.sv'
import { i18n, setLanguage, t } from '@/i18n'
import { TOUR_STEPS } from './tour'

type Tree = { [k: string]: string | Tree }
function flatten(obj: Tree, prefix = '', out: Record<string, string> = {}): Record<string, string> {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out[key] = v
    else flatten(v, key, out)
  }
  return out
}
const EN = flatten(guideEn as unknown as Tree)
const SV = flatten(guideSv as unknown as Tree)
const placeholders = (s: string) => [...new Set([...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort().join()

describe('guide texts en / sv', () => {
  it('have the same keys', () => {
    expect(Object.keys(SV).sort()).toEqual(Object.keys(EN).sort())
  })
  it('are not empty and use the same placeholders and plural forms', () => {
    for (const k of Object.keys(EN)) {
      expect(EN[k].trim(), k).not.toBe('')
      expect(SV[k].trim(), k).not.toBe('')
      expect(placeholders(SV[k]), k).toBe(placeholders(EN[k]))
      expect(SV[k].split(' | ').length, k).toBe(EN[k].split(' | ').length)
    }
  })
  it('avoid characters with special meaning in vue-i18n', () => {
    const bad = [...Object.entries(EN), ...Object.entries(SV)].filter(([, v]) => /[@$]/.test(v)).map(([k]) => k)
    expect(bad).toEqual([])
  })
  it('cover every tour step', () => {
    for (const s of TOUR_STEPS) {
      expect(EN[`tour.steps.${s.id}.title`], s.id).toBeTruthy()
      expect(EN[`tour.steps.${s.id}.body`], s.id).toBeTruthy()
    }
    expect(Object.keys(EN).filter((k) => k.startsWith('tour.steps.')).length).toBe(TOUR_STEPS.length * 2)
  })
  it('are merged into the app messages, with the help page title', () => {
    setLanguage('sv')
    expect(t('guide.userGuide')).toBe('Användarhandbok')
    expect(t('titles.help')).toBe('Hjälp')
    expect(t('titles.projects')).toBe('Projekt')
    setLanguage('en')
    expect(t('tour.stepOf', { n: 2, total: 9 })).toBe('Step 2 of 9')
    expect(i18n.global.te('titles.login')).toBe(true)
  })
})
