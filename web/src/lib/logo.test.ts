import { describe, expect, it } from 'vitest'
import { LOGO_MAX_BYTES, logoProblem, logoSizeMM } from './logo'

describe('logo', () => {
  it('fits the logo into the maximum size without enlarging it', () => {
    // 960 × 480 px = 254 × 127 mm at 96 dpi; box 50 × 20 → 40 × 20
    const s = logoSizeMM({ width: 960, height: 480, max_width_mm: 50, max_height_mm: 20 })
    expect(s.w).toBeCloseTo(40)
    expect(s.h).toBeCloseTo(20)
    // 96 px = 25.4 mm: not enlarged to the box
    const small = logoSizeMM({ width: 96, height: 96, max_width_mm: 100, max_height_mm: 40 })
    expect(small.w).toBeCloseTo(25.4)
    // out-of-range boxes are clamped like the server's
    expect(logoSizeMM({ width: 10000, height: 100, max_width_mm: 1000, max_height_mm: 1 }).w).toBeCloseTo(170)
    expect(logoSizeMM({ width: 0, height: 10, max_width_mm: 50, max_height_mm: 20 })).toEqual({ w: 0, h: 0 })
  })
  it('checks uploads by type, name and size', () => {
    expect(logoProblem({ size: 10, type: 'image/png', name: 'a.png' })).toBeNull()
    expect(logoProblem({ size: 10, type: '', name: 'logo.SVG' })).toBeNull()
    expect(logoProblem({ size: 10, type: 'image/svg+xml', name: 'x' })).toBeNull()
    expect(logoProblem({ size: 10, type: 'image/gif', name: 'a.gif' })).toBe('type')
    expect(logoProblem({ size: 0, type: 'image/png' })).toBe('empty')
    expect(logoProblem({ size: LOGO_MAX_BYTES + 1, type: 'image/png' })).toBe('too_large')
  })
})
