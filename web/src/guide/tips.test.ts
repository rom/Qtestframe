// The context-sensitive help tips: every tip has a text in both languages,
// points at an existing guide page and at a heading that exists in each
// language, and every tip used in the code is registered (and vice versa).
import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import guideEn from '@/i18n/locales/guide.en'
import guideSv from '@/i18n/locales/guide.sv'
import { getPage } from './content'
import { GUIDE_PAGES } from './pages'
import { renderGuide } from './render'
import { placeTip, queryGuidePath, TIP_IDS, TIPS, tipHelpPath, tipKey, type TipTarget } from './tips'

const LANGS = { en: guideEn, sv: guideSv } as const

function anchors(lang: string, page: string): Set<string> {
  const p = getPage(lang, page)!
  return new Set([...renderGuide(p.markdown, { lang, assets: {} }).html.matchAll(/ id="([^"]+)"/g)].map((m) => m[1]))
}

function sources(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) sources(p, out)
    else if (/\.vue$/.test(name)) out.push(p)
  }
  return out
}

describe('help tips', () => {
  it('have a non-empty text in every language', () => {
    for (const [lang, msgs] of Object.entries(LANGS)) {
      const tips = (msgs as unknown as { tips: Record<string, string> }).tips
      for (const id of TIP_IDS) expect(tips[tipKey(id).slice(5)]?.trim(), `${lang}: ${id}`).toBeTruthy()
      expect(Object.keys(tips).sort(), lang).toEqual(TIP_IDS.map((id) => tipKey(id).slice(5)).sort())
    }
  })
  it('link to existing pages and headings in every language', () => {
    const broken: string[] = []
    for (const id of TIP_IDS) {
      const target: TipTarget = TIPS[id]
      expect(GUIDE_PAGES as readonly string[]).toContain(target.page)
      for (const lang of Object.keys(LANGS)) {
        const anchor = target.anchor[lang]
        if (!anchor || !anchors(lang, target.page).has(anchor)) broken.push(`${lang}: ${id} → ${target.page}#${anchor}`)
      }
    }
    expect(broken).toEqual([])
  })
  it('are all used in the client, and only registered ones are used', () => {
    const used = new Set<string>()
    for (const f of sources(join(process.cwd(), 'src'))) {
      for (const m of readFileSync(f, 'utf8').matchAll(/<HelpTip[^>]*\sid="([^"]+)"/g)) used.add(m[1])
    }
    used.add('query.filter') // also the guide link of the query help
    expect([...used].filter((id) => !(id in TIPS))).toEqual([])
    expect(TIP_IDS.filter((id) => !used.has(id))).toEqual([])
  })
  it('build the guide path for a language', () => {
    expect(tipHelpPath('req.obligation', 'en')).toBe('/help/requirements#obligation-levels')
    expect(tipHelpPath('req.obligation', 'sv')).toBe('/help/requirements#' + encodeURIComponent('kravnivåer'))
    expect(tipHelpPath('req.obligation', 'de')).toBe('/help/requirements')
    expect(tipHelpPath('nope', 'en')).toBeNull()
    expect(tipKey('req.obligation')).toBe('tips.req_obligation')
    expect(queryGuidePath('en')).toBe('/help/search-and-filters#syntax-in-five-minutes')
  })
})

describe('placeTip', () => {
  const size = { width: 200, height: 80 }
  it('goes below the trigger, centred and inside the viewport', () => {
    expect(placeTip({ top: 100, bottom: 120, left: 300, width: 20 }, size, 1000, 800)).toEqual({ top: 126, left: 210, above: false })
    expect(placeTip({ top: 100, bottom: 120, left: 2, width: 20 }, size, 1000, 800).left).toBe(8)
    expect(placeTip({ top: 100, bottom: 120, left: 990, width: 20 }, size, 1000, 800).left).toBe(792)
  })
  it('goes above when there is no room below', () => {
    expect(placeTip({ top: 700, bottom: 720, left: 300, width: 20 }, size, 1000, 760)).toEqual({ top: 614, left: 210, above: true })
  })
  it('stays inside a viewport that is too small', () => {
    const p = placeTip({ top: 10, bottom: 30, left: 10, width: 20 }, { width: 400, height: 500 }, 300, 200)
    expect(p.left).toBe(8)
    expect(p.top).toBeGreaterThanOrEqual(8)
  })
})
