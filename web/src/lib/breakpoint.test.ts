import { describe, expect, it } from 'vitest'
import { effectScope, nextTick } from 'vue'
import { BREAKPOINTS, maxWidthQuery, useBreakpoint, useMediaQuery } from './breakpoint'

function fakeMatchMedia(initial: boolean) {
  const listeners = new Set<(e: Event) => void>()
  const queries: string[] = []
  const mm = (q: string) => {
    queries.push(q)
    return {
      matches: initial,
      addEventListener: (_: string, fn: (e: Event) => void) => listeners.add(fn),
      removeEventListener: (_: string, fn: (e: Event) => void) => listeners.delete(fn),
    } as unknown as MediaQueryList
  }
  const fire = (matches: boolean) => listeners.forEach((fn) => fn({ matches } as unknown as Event))
  return { mm, fire, listeners, queries }
}

describe('breakpoints', () => {
  it('builds max-width queries', () => {
    expect(maxWidthQuery('phone')).toBe(`(max-width: ${BREAKPOINTS.phone}px)`)
    expect(maxWidthQuery('tablet')).toBe('(max-width: 1023px)')
    expect(maxWidthQuery(767)).toBe('(max-width: 767px)')
  })
  it('follows the media query and cleans up with its scope', async () => {
    const f = fakeMatchMedia(true)
    const scope = effectScope()
    const m = scope.run(() => useBreakpoint('phone', f.mm))!
    expect(f.queries).toEqual(['(max-width: 640px)'])
    expect(m.value).toBe(true)
    f.fire(false)
    await nextTick()
    expect(m.value).toBe(false)
    expect(f.listeners.size).toBe(1)
    scope.stop()
    expect(f.listeners.size).toBe(0)
  })
  it('works outside a scope and without matchMedia', () => {
    const f = fakeMatchMedia(false)
    expect(useMediaQuery('(min-width: 1px)', true, f.mm).value).toBe(false)
    expect(useMediaQuery('(x)', true, null).value).toBe(true)
    expect(useBreakpoint('tablet', null).value).toBe(false)
    const throwing = () => {
      throw new Error('bad query')
    }
    expect(useMediaQuery('(x)', true, throwing as never).value).toBe(true)
    // the default uses window.matchMedia when there is one
    const prev = window.matchMedia
    window.matchMedia = f.mm as never
    expect(useBreakpoint('phone').value).toBe(false)
    window.matchMedia = prev
  })
})
