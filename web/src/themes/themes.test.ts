import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { aaMinimum, blend, contrastRatio, luminance, meetsAA, parseColor } from './contrast'
import {
  concreteTheme,
  contrastPairs,
  DECORATIVE_THEMES,
  isDecorative,
  isThemeId,
  schemeOf,
  THEME_DEFS,
  THEME_IDS,
  THEME_LABELS,
  THEME_TEXT,
  type ConcreteThemeId,
} from './registry'
import { presetFor, presetKey, presetTokens } from './presets'
import { applyTheme, bootTheme, currentPresetKey, localTheme, resetThemeState } from './apply'
import * as index from './index'

const ALL = ['light', 'dark', 'system', 'spring', 'autumn', 'desert', 'camouflage', 'city_camouflage', 'navy', 'green_console', 'crazy', 'ocean', 'forest', 'sunset', 'nordic', 'lavender', 'high_contrast']
const DARK = ['dark', 'camouflage', 'city_camouflage', 'navy', 'green_console', 'forest', 'high_contrast']
const PAINTED = THEME_IDS.filter((t) => t !== 'system') as ConcreteThemeId[]

describe('contrast', () => {
  it('parses colours', () => {
    expect(parseColor('#fff')).toEqual({ r: 255, g: 255, b: 255, a: 1 })
    expect(parseColor('#00000080').a).toBeCloseTo(0.5, 2)
    expect(parseColor('#1234')).toEqual({ r: 0x11, g: 0x22, b: 0x33, a: 0x44 / 255 })
    expect(parseColor('rgb(1, 2, 3)')).toEqual({ r: 1, g: 2, b: 3, a: 1 })
    expect(parseColor('rgba(1,2,3,0.25)').a).toBe(0.25)
    expect(parseColor('rgb(1 2 3 / 50%)').a).toBe(0.5)
    expect(() => parseColor('red')).toThrow()
  })
  it('computes WCAG ratios', () => {
    expect(contrastRatio('#000', '#fff')).toBeCloseTo(21, 5)
    expect(contrastRatio('#fff', '#fff')).toBe(1)
    expect(contrastRatio('#777777', '#ffffff')).toBeCloseTo(4.48, 2)
    expect(contrastRatio('#ffffff', '#777777')).toBeCloseTo(contrastRatio('#777777', '#ffffff'), 10)
    expect(luminance('#ffffff')).toBeCloseTo(1, 10)
    expect(luminance(parseColor('#000'))).toBe(0)
    expect(contrastRatio(parseColor('#000'), parseColor('#fff'))).toBeCloseTo(21, 5)
  })
  it('blends translucent colours', () => {
    expect(blend('rgba(0,0,0,0.5)', '#ffffff')).toEqual({ r: 128, g: 128, b: 128, a: 1 })
    expect(blend(parseColor('#000'), parseColor('#fff'))).toEqual({ r: 0, g: 0, b: 0, a: 1 })
    expect(contrastRatio('rgba(0,0,0,0)', '#ffffff')).toBe(1)
  })
  it('knows the AA thresholds', () => {
    expect(aaMinimum('text')).toBe(4.5)
    expect(aaMinimum('large')).toBe(3)
    expect(aaMinimum('ui')).toBe(3)
    expect(meetsAA('#767676', '#ffffff')).toBe(true)
    expect(meetsAA('#777777', '#ffffff')).toBe(false)
    expect(meetsAA('#777777', '#ffffff', 'large')).toBe(true)
  })
})

describe('theme registry', () => {
  it('covers every preference value', () => {
    expect([...THEME_IDS]).toEqual(ALL)
    expect(Object.keys(THEME_DEFS).sort()).toEqual([...ALL].sort())
    expect([...DECORATIVE_THEMES]).toEqual(ALL.slice(3))
    for (const id of THEME_IDS) expect(THEME_DEFS[id].id).toBe(id)
  })
  it('has a label for every theme', () => {
    expect(Object.keys(THEME_LABELS).sort()).toEqual([...ALL].sort())
    for (const id of THEME_IDS) expect(THEME_LABELS[id].length).toBeGreaterThan(0)
    expect(new Set(Object.values(THEME_LABELS)).size).toBe(ALL.length)
    expect(THEME_TEXT.picker).toBeTruthy()
    expect(THEME_TEXT.unset).toBeTruthy()
  })
  it('declares light/dark schemes', () => {
    for (const id of PAINTED) expect(THEME_DEFS[id].scheme).toBe(DARK.includes(id) ? 'dark' : 'light')
    expect(THEME_DEFS.system.scheme).toBeNull()
    expect(schemeOf('system', true)).toBe('dark')
    expect(schemeOf('system', false)).toBe('light')
    expect(schemeOf('system')).toBe('light')
    expect(schemeOf('navy')).toBe('dark')
    expect(concreteTheme('system', true)).toBe('dark')
    expect(concreteTheme('system')).toBe('light')
    expect(concreteTheme('crazy', true)).toBe('crazy')
  })
  it('has complete palettes', () => {
    for (const id of PAINTED) {
      const d = THEME_DEFS[id]
      expect(d.colors, id).toBeDefined()
      expect(Object.keys(d.primary!)).toHaveLength(11)
      if (d.decorative) expect(Object.keys(d.surface!)).toHaveLength(12)
      for (const v of Object.values(d.colors!).flat()) expect(() => parseColor(v as string), `${id} ${v}`).not.toThrow()
    }
    expect(THEME_DEFS.green_console.mono).toBe(true)
  })
  it('validates ids', () => {
    expect(isThemeId('navy')).toBe(true)
    expect(isThemeId('system')).toBe(true)
    expect(isThemeId('pink')).toBe(false)
    expect(isThemeId(3)).toBe(false)
    expect(isDecorative('navy')).toBe(true)
    expect(isDecorative('dark')).toBe(false)
    expect(isDecorative(null)).toBe(false)
  })
})

describe('WCAG AA contrast of every theme', () => {
  for (const id of PAINTED) {
    it(id, () => {
      const failures = contrastPairs(THEME_DEFS[id].colors!)
        .map((p) => ({ ...p, ratio: contrastRatio(p.fg, p.bg) }))
        .filter((p) => p.ratio < aaMinimum(p.kind))
        .map((p) => `${p.name}: ${p.fg} on ${p.bg} = ${p.ratio.toFixed(2)}`)
      expect(failures).toEqual([])
    })
  }
  it('checks pattern colours too', () => {
    const names = contrastPairs(THEME_DEFS.camouflage.colors!).map((p) => p.name)
    expect(names.some((n) => n.startsWith('sidebar text on pattern'))).toBe(true)
    expect(names.some((n) => n.startsWith('top bar text on pattern'))).toBe(true)
  })
})

describe('styles.css mirrors the palettes', () => {
  const css = readFileSync(resolve(__dirname, '../styles.css'), 'utf8')
  const block = (sel: string) => {
    const i = css.indexOf(sel + ' {')
    expect(i, sel).toBeGreaterThan(-1)
    return css.slice(i, css.indexOf('}', i))
  }
  const value = (b: string, v: string) => new RegExp(`${v}:\\s*([^;]+);`).exec(b)?.[1].trim()
  for (const id of DECORATIVE_THEMES) {
    it(id, () => {
      const b = block(`:root[data-theme="${id}"]`)
      const c = THEME_DEFS[id].colors!
      const map: Record<string, string> = {
        '--qtf-bg': c.bg,
        '--qtf-card': c.card,
        '--qtf-text': c.text,
        '--qtf-muted': c.muted,
        '--qtf-border': c.border,
        '--qtf-sidebar-bg': c.sidebarBg,
        '--qtf-sidebar-text': c.sidebarText,
        '--qtf-sidebar-heading': c.sidebarHeading,
        '--qtf-sidebar-strong': c.sidebarStrong,
        '--qtf-sidebar-active': c.sidebarActiveBg,
        '--qtf-sidebar-active-text': c.sidebarActiveText,
        '--qtf-topbar-bg': c.topbarBg,
        'color-scheme': THEME_DEFS[id].scheme!,
      }
      for (const [k, v] of Object.entries(map)) expect(value(b, k), `${id} ${k}`).toBe(v)
    })
  }
  it('base light and dark', () => {
    const root = block(':root')
    const dark = block(':root.app-dark')
    for (const k of ['bg', 'card', 'text', 'muted'] as const) {
      expect(value(root, `--qtf-${k}`)).toBe(THEME_DEFS.light.colors![k])
      expect(value(dark, `--qtf-${k}`)).toBe(THEME_DEFS.dark.colors![k])
    }
    expect(value(root, '--qtf-sidebar-active')).toBe(THEME_DEFS.light.colors!.sidebarActiveBg)
    expect(value(dark, '--qtf-sidebar-active')).toBe(THEME_DEFS.dark.colors!.sidebarActiveBg)
  })
  it('disables theme motion under prefers-reduced-motion', () => {
    expect(css).toContain('@media (prefers-reduced-motion: reduce)')
  })
})

describe('presets', () => {
  it('shares one preset between light and dark', () => {
    expect(presetKey('light')).toBe('base')
    expect(presetKey('dark')).toBe('base')
    expect(presetKey('navy')).toBe('navy')
    expect(presetFor('light')).toBe(presetFor('dark'))
    expect(presetFor('navy')).toBe(presetFor('navy'))
    expect(presetFor('navy')).not.toBe(presetFor('spring'))
  })
  it('puts the palette into the active colour scheme', () => {
    for (const id of DECORATIVE_THEMES) {
      const d = THEME_DEFS[id]
      const t = presetTokens(d) as { semantic: { primary: unknown; colorScheme: Record<string, { primary?: { color: string; contrastColor: string }; text?: { color: string } }> } }
      expect(t.semantic.primary).toBe(d.primary)
      const s = t.semantic.colorScheme[d.scheme!]
      expect(s.primary!.color).toBe(d.colors!.primary)
      expect(s.primary!.contrastColor).toBe(d.colors!.onPrimary)
      expect(s.text!.color).toBe(d.colors!.text)
    }
    expect((presetTokens(THEME_DEFS.crazy) as { primitive?: unknown }).primitive).toBeDefined()
    expect((presetTokens(THEME_DEFS.navy) as { primitive?: unknown }).primitive).toBeUndefined()
  })
})

describe('applyTheme', () => {
  afterEach(() => {
    resetThemeState()
    document.documentElement.className = ''
    document.documentElement.removeAttribute('data-theme')
    document.head.innerHTML = ''
    localStorage.clear()
  })

  it('paints the document and switches presets only when needed', () => {
    const setPreset = vi.fn()
    applyTheme('light', { setPreset })
    expect(setPreset).not.toHaveBeenCalled() // first call: preset installed by main.ts
    expect(currentPresetKey()).toBe('base')
    const root = document.documentElement
    expect(root.dataset.theme).toBe('light')
    expect(root.classList.contains('app-dark')).toBe(false)
    expect(root.style.colorScheme).toBe('light')

    applyTheme('dark', { setPreset })
    expect(setPreset).not.toHaveBeenCalled()
    expect(root.classList.contains('app-dark')).toBe(true)

    applyTheme('navy', { setPreset })
    expect(setPreset).toHaveBeenCalledWith(presetFor('navy'))
    expect(root.dataset.theme).toBe('navy')
    expect(root.classList.contains('app-dark')).toBe(true)
    expect(root.style.colorScheme).toBe('dark')
    expect(document.querySelectorAll('meta[name="theme-color"]')).toHaveLength(1)
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')!.content).toBe(THEME_DEFS.navy.colors!.topbarBg)

    applyTheme('spring', { setPreset })
    expect(root.classList.contains('app-dark')).toBe(false)
    expect(document.querySelectorAll('meta[name="theme-color"]')).toHaveLength(1)
    applyTheme('spring', { setPreset })
    expect(setPreset).toHaveBeenCalledTimes(2)
  })

  it('reads the local theme', () => {
    const kv = (m: Record<string, string>) => ({ getItem: (k: string) => m[k] ?? null })
    expect(localTheme(kv({ 'qtf.decor': 'navy', 'qtf.theme': 'dark' }), false)).toBe('navy')
    expect(localTheme(kv({ 'qtf.theme': 'dark' }), false)).toBe('dark')
    expect(localTheme(kv({}), true)).toBe('dark')
    expect(localTheme(null, false)).toBe('light')
    const broken = {
      getItem: () => {
        throw new Error('denied')
      },
    }
    expect(localTheme(broken, true)).toBe('dark')
    localStorage.setItem('qtf.decor', 'crazy')
    expect(localTheme()).toBe('crazy')
  })

  it('boots with the local theme and its preset', () => {
    const b = bootTheme({ store: { getItem: (k) => (k === 'qtf.decor' ? 'green_console' : null) }, systemDark: false })
    expect(b.id).toBe('green_console')
    expect(b.preset).toBe(presetFor('green_console'))
    expect(document.documentElement.dataset.theme).toBe('green_console')
    expect(currentPresetKey()).toBe('green_console')
    localStorage.setItem('qtf.theme', 'dark')
    expect(bootTheme().id).toBe('dark')
    expect(bootTheme({ store: null, systemDark: true }).id).toBe('dark')
  })

  it('re-exports from the index', () => {
    expect(index.THEME_LABELS).toBe(THEME_LABELS)
    expect(typeof index.applyTheme).toBe('function')
    expect(typeof index.contrastRatio).toBe('function')
  })
})

describe('theme scales', () => {
  it('holds only valid six-digit colours', () => {
    for (const id of THEME_IDS) {
      if (id === 'system') continue
      const def = THEME_DEFS[id]
      for (const scale of [def.primary, def.surface]) {
        if (!scale) continue
        for (const v of Object.values(scale)) expect(v, `${id}: ${v}`).toMatch(/^#[0-9a-f]{6}$/i)
      }
    }
  })
})
