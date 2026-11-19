import { beforeEach, describe, expect, it } from 'vitest'
import type { Me } from '@/lib/types'
import {
  TOUR_STEPS,
  focusables,
  initialTour,
  isLastStep,
  placeCard,
  resolveTarget,
  shouldStartTour,
  spotlight,
  tourEnded,
  tourTransition,
  trapFocus,
  type TourState,
} from './tour'

const me = (prefs: Record<string, unknown> = {}, first?: boolean) => ({ user: { id: 'u', preferences: prefs }, first_login: first }) as unknown as Me

describe('steps', () => {
  it('has 7–9 steps with unique ids, starting with a welcome card', () => {
    expect(TOUR_STEPS.length).toBeGreaterThanOrEqual(7)
    expect(TOUR_STEPS.length).toBeLessThanOrEqual(9)
    expect(new Set(TOUR_STEPS.map((s) => s.id)).size).toBe(TOUR_STEPS.length)
    expect(TOUR_STEPS[0].targets).toEqual([])
    for (const id of ['navigation', 'projects', 'requirements', 'tests', 'operations', 'search', 'profile', 'help']) expect(TOUR_STEPS.some((s) => s.id === id)).toBe(true)
  })
  it('has valid selectors', () => {
    for (const s of TOUR_STEPS) for (const sel of s.targets) expect(() => document.querySelector(sel)).not.toThrow()
  })
})

describe('shouldStartTour', () => {
  it('starts on first sign-in', () => {
    expect(shouldStartTour(me({ intro_done: true }, true))).toBe(true)
  })
  it('starts until the tour was finished or skipped', () => {
    expect(shouldStartTour(me())).toBe(true)
    expect(shouldStartTour(me({ intro_done: false }, false))).toBe(true)
    expect(shouldStartTour(me({ intro_done: true }, false))).toBe(false)
  })
  it('never starts without a user', () => {
    expect(shouldStartTour(null)).toBe(false)
    expect(shouldStartTour({} as Me)).toBe(false)
  })
})

describe('tourTransition', () => {
  const run = (events: Parameters<typeof tourTransition>[1][], total = 3) => events.reduce((s, e) => tourTransition(s, e), initialTour(total))
  it('starts at the first step', () => {
    expect(run(['start'])).toEqual({ status: 'active', index: 0, total: 3 })
    expect(initialTour().total).toBe(TOUR_STEPS.length)
  })
  it('moves forward and back within bounds', () => {
    expect(run(['start', 'next', 'next']).index).toBe(2)
    expect(run(['start', 'back']).index).toBe(0)
    expect(run(['start', 'next', 'back']).index).toBe(0)
  })
  it('finishes after the last step', () => {
    const s = run(['start', 'next', 'next'])
    expect(isLastStep(s)).toBe(true)
    const done = tourTransition(s, 'next')
    expect(done.status).toBe('finished')
    expect(tourEnded(done)).toBe(true)
    expect(isLastStep(done)).toBe(false)
  })
  it('can be skipped at any step', () => {
    const s = run(['start', 'next', 'skip'])
    expect(s.status).toBe('skipped')
    expect(tourEnded(s)).toBe(true)
  })
  it('ignores events while not active', () => {
    const idle = initialTour(3)
    for (const e of ['next', 'back', 'skip'] as const) expect(tourTransition(idle, e)).toBe(idle)
    expect(tourEnded(idle)).toBe(false)
  })
  it('can be restarted after it ended', () => {
    expect(run(['start', 'skip', 'start'])).toEqual({ status: 'active', index: 0, total: 3 })
  })
  it('ends at once without steps', () => {
    const empty: TourState = initialTour(0)
    expect(tourTransition(empty, 'start').status).toBe('finished')
    expect(tourTransition(empty, 'next')).toEqual(empty)
  })
})

describe('resolveTarget', () => {
  beforeEach(() => {
    document.body.innerHTML = `
    <aside id="app-sidebar"><nav><a class="nav-item" href="/projects">P</a></nav></aside>
    <header class="topbar"><button class="user-button">me</button><button class="hidden-one">x</button></header>`
  })
  const visibleAll = () => true
  it('takes the first matching, visible selector', () => {
    const r = resolveTarget({ id: 'x', targets: ['.nope', '.topbar .user-button'] }, document, { isVisible: visibleAll })
    expect(r?.el.className).toBe('user-button')
    expect(r?.inDrawer).toBe(false)
  })
  it('flags targets in the drawer and accepts them while it is closed', () => {
    const step = { id: 'p', targets: ['#app-sidebar a.nav-item[href$="/projects"]'] }
    expect(resolveTarget(step, document, { isVisible: () => false })).toBeNull()
    const r = resolveTarget(step, document, { drawerClosed: true, isVisible: () => false })
    expect(r?.inDrawer).toBe(true)
  })
  it('skips invisible and invalid targets', () => {
    expect(resolveTarget({ id: 'x', targets: ['[[bad', '.hidden-one'] }, document)).toBeNull()
    expect(resolveTarget({ id: 'x', targets: [] }, document)).toBeNull()
  })
  it('uses layout and styles to decide visibility by default', () => {
    const el = document.querySelector('.user-button') as HTMLElement
    el.getBoundingClientRect = () => ({ width: 40, height: 30, top: 0, left: 0, right: 40, bottom: 30, x: 0, y: 0, toJSON: () => ({}) })
    expect(resolveTarget({ id: 'x', targets: ['.user-button'] }, document)?.el).toBe(el)
    el.style.visibility = 'hidden'
    expect(resolveTarget({ id: 'x', targets: ['.user-button'] }, document)).toBeNull()
  })
})

describe('spotlight and placeCard', () => {
  const vp = { width: 1440, height: 900 }
  const card = { width: 380, height: 220 }
  it('pads the hole and keeps it on screen', () => {
    expect(spotlight({ top: 100, left: 100, width: 50, height: 20 }, vp)).toEqual({ top: 94, left: 94, width: 62, height: 32 })
    expect(spotlight({ top: 0, left: 0, width: 2000, height: 10 }, vp, 6)).toEqual({ top: 2, left: 2, width: 1436, height: 14 })
  })
  it('centres without a target', () => {
    expect(placeCard(null, card, vp)).toEqual({ top: 340, left: 530, placement: 'center' })
  })
  it('prefers the right side, then below, above and left', () => {
    expect(placeCard({ top: 100, left: 10, width: 200, height: 30 }, card, vp).placement).toBe('right')
    expect(placeCard({ top: 10, left: 1200, width: 200, height: 40 }, card, vp)).toMatchObject({ placement: 'bottom', top: 64, left: 1048 })
    expect(placeCard({ top: 800, left: 1200, width: 200, height: 40 }, card, vp)).toMatchObject({ placement: 'top', top: 566 })
    expect(placeCard({ top: 100, left: 900, width: 520, height: 790 }, card, vp)).toMatchObject({ placement: 'left', left: 506 })
    expect(placeCard({ top: 0, left: 0, width: 1440, height: 900 }, card, vp).placement).toBe('center')
  })
  it('spans the width on phones, below or above the target', () => {
    const phone = { width: 390, height: 844 }
    const c = { width: 366, height: 240 }
    expect(placeCard({ top: 10, left: 300, width: 40, height: 40 }, c, phone)).toEqual({ top: 64, left: 12, placement: 'bottom' })
    expect(placeCard({ top: 700, left: 10, width: 40, height: 40 }, c, phone)).toEqual({ top: 446, left: 12, placement: 'top' })
    expect(placeCard({ top: 200, left: 0, width: 390, height: 500 }, c, phone).placement).toBe('center')
  })
})

describe('focus trap', () => {
  let items: HTMLElement[] = []
  beforeEach(() => {
    document.body.innerHTML = '<div id="c"><h2 tabindex="-1">t</h2><button>a</button><button disabled>x</button><a href="#">b</a><button>c</button></div>'
    items = focusables(document.getElementById('c')!)
  })
  it('lists focusable elements only', () => {
    expect(items.map((e) => e.textContent)).toEqual(['a', 'b', 'c'])
  })
  it('wraps around at both ends', () => {
    expect(trapFocus(items, items[2], false)).toBe(items[0])
    expect(trapFocus(items, items[0], true)).toBe(items[2])
    expect(trapFocus(items, items[1], false)).toBeNull()
    expect(trapFocus(items, items[1], true)).toBeNull()
  })
  it('pulls focus back in from outside', () => {
    expect(trapFocus(items, document.body, false)).toBe(items[0])
    expect(trapFocus(items, null, true)).toBe(items[2])
    expect(trapFocus([], null, false)).toBeNull()
  })
})
