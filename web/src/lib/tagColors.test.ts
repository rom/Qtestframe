import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { contrastRatio } from '@/themes/contrast'
import { TAG_COLOR_COUNT, TAG_PALETTE, tagClass, tagColorIndex } from './tagColors'

describe('tag colours', () => {
  it('are stable and case-insensitive', () => {
    expect(tagColorIndex('GDPR')).toBe(tagColorIndex('gdpr'))
    expect(tagColorIndex(' login ')).toBe(tagColorIndex('login'))
    expect(tagClass('login')).toMatch(/^tag-c\d$/)
  })
  it('spread tags over the palette', () => {
    const used = new Set(['login', 'security', 'payment', 'gdpr', 'smoke', 'regression', 'api', 'ui', 'performance', 'lockout', 'export', 'audit', 'sso', 'mobile', 'åtkomst'].map(tagColorIndex))
    expect(used.size).toBeGreaterThanOrEqual(6)
    for (const i of used) expect(i).toBeLessThan(TAG_COLOR_COUNT)
  })
  it('keep AA text contrast in light and dark', () => {
    expect(TAG_PALETTE).toHaveLength(TAG_COLOR_COUNT)
    for (const p of TAG_PALETTE) {
      expect(contrastRatio(p.light[1], p.light[0])).toBeGreaterThanOrEqual(4.5)
      expect(contrastRatio(p.dark[1], p.dark[0])).toBeGreaterThanOrEqual(4.5)
    }
  })
  it('match styles.css', () => {
    const css = readFileSync(resolve(__dirname, '../styles.css'), 'utf8')
    TAG_PALETTE.forEach((p, i) => {
      expect(css).toContain(`.tag-chip.tag-c${i}, .tag-colored.tag-c${i} { background: ${p.light[0]}; color: ${p.light[1]};`)
      expect(css).toContain(`.app-dark .tag-chip.tag-c${i}, .app-dark .tag-colored.tag-c${i} { background: ${p.dark[0]}; color: ${p.dark[1]};`)
    })
  })
})
