import { describe, expect, it } from 'vitest'
import { chartColors, MONO_FONT, SANS_FONT, seriesColor } from './chart'
import { THEME_DEFS, THEME_IDS, type ConcreteThemeId } from './registry'
import { contrastRatio } from './contrast'
import * as index from './index'

const PAINTED = THEME_IDS.filter((t) => t !== 'system') as ConcreteThemeId[]

describe('chart colours', () => {
  it('follow every painted theme', () => {
    for (const id of PAINTED) {
      const c = chartColors(id)
      const def = THEME_DEFS[id]
      expect(c.text).toBe(def.colors!.muted)
      expect(c.grid).toBe(def.colors!.border)
      expect(c.surface).toBe(def.colors!.card)
      expect(c.series[0]).toBe(def.colors!.primary)
      expect(c.series.length).toBeGreaterThanOrEqual(6)
      expect(c.dark).toBe(def.scheme === 'dark')
      // tick and legend text stays readable on the card
      expect(contrastRatio(c.text, c.surface)).toBeGreaterThanOrEqual(4.5)
    }
  })
  it('use the console font only in green_console', () => {
    expect(chartColors('green_console').font).toBe(MONO_FONT)
    expect(chartColors('light').font).toBe(SANS_FONT)
    expect(chartColors('navy').font).toBe(SANS_FONT)
  })
  it('differ between themes and fall back to light', () => {
    expect(chartColors('light').series).not.toEqual(chartColors('navy').series)
    expect(chartColors('bogus' as ConcreteThemeId).text).toBe(chartColors('light').text)
  })
  it('wraps series colours', () => {
    const c = { series: ['#a', '#b', '#c'] }
    expect(seriesColor(c, 0)).toBe('#a')
    expect(seriesColor(c, 4)).toBe('#b')
    expect(seriesColor(c, -2)).toBe('#c')
  })
  it('is exported from the themes index', () => {
    expect(index.chartColors).toBe(chartColors)
  })
})
