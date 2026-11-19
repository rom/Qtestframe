// The first-login intro tour: its steps, when it starts, its state machine,
// and where the spotlight and the step card go. The component
// (IntroTour.vue) only wires these to the DOM.
import type { Me } from '@/lib/types'

export interface TourStep {
  /** message keys tour.steps.<id>.title / .body */
  id: string
  /** CSS selectors of the element to highlight, tried in order; none = centred card */
  targets: readonly string[]
}

/** The steps, in order. Targets that are missing (e.g. outside a project) fall back to a centred card. */
export const TOUR_STEPS: readonly TourStep[] = [
  { id: 'welcome', targets: [] },
  { id: 'navigation', targets: ['#app-sidebar nav'] },
  { id: 'projects', targets: ['#app-sidebar a.nav-item[href$="/projects"]'] },
  { id: 'requirements', targets: ['#app-sidebar a.nav-item[href$="/requirements"]'] },
  { id: 'tests', targets: ['#app-sidebar a.nav-item[href$="/tests"]'] },
  { id: 'operations', targets: ['#app-sidebar a.nav-item[href$="/runs"]'] },
  { id: 'search', targets: ['.topbar form[role="search"]', '.topbar button[aria-haspopup="dialog"]', '#app-sidebar a.nav-item[href$="/search"]'] },
  { id: 'profile', targets: ['.topbar .user-button'] },
  { id: 'help', targets: ['.topbar .qtf-help-button'] },
]

/**
 * Whether the tour should start for this user: until it was finished or
 * skipped (preference intro_done). A first sign-in has no intro_done yet, so
 * it starts then; once intro_done is stored (by the tour, or by an
 * administrator or script via the API) it never starts automatically.
 */
export function shouldStartTour(me: Pick<Me, 'user' | 'first_login'> | null | undefined): boolean {
  if (!me?.user) return false
  return me.user.preferences?.intro_done !== true
}

// ---------- state machine ----------

export type TourStatus = 'idle' | 'active' | 'finished' | 'skipped'

export interface TourState {
  status: TourStatus
  index: number
  total: number
}

export type TourEvent = 'start' | 'next' | 'back' | 'skip'

export function initialTour(total = TOUR_STEPS.length): TourState {
  return { status: 'idle', index: 0, total }
}

/** The next state; events that do not apply leave the state unchanged. */
export function tourTransition(state: TourState, event: TourEvent): TourState {
  if (state.total <= 0) return { ...state, status: event === 'start' ? 'finished' : state.status }
  switch (event) {
    case 'start':
      return { ...state, status: 'active', index: 0 }
    case 'next':
      if (state.status !== 'active') return state
      return state.index >= state.total - 1 ? { ...state, status: 'finished' } : { ...state, index: state.index + 1 }
    case 'back':
      if (state.status !== 'active' || state.index === 0) return state
      return { ...state, index: state.index - 1 }
    case 'skip':
      return state.status === 'active' ? { ...state, status: 'skipped' } : state
  }
}

/** The tour has ended (finished or skipped): the moment to store intro_done. */
export function tourEnded(state: TourState): boolean {
  return state.status === 'finished' || state.status === 'skipped'
}

export const isLastStep = (s: TourState) => s.status === 'active' && s.index === s.total - 1

// ---------- target resolution ----------

export interface Box {
  top: number
  left: number
  width: number
  height: number
}

function defaultVisible(el: Element): boolean {
  const r = el.getBoundingClientRect()
  if (r.width <= 0 || r.height <= 0) return false
  const cs = el.ownerDocument.defaultView?.getComputedStyle(el)
  return !cs || (cs.visibility !== 'hidden' && cs.display !== 'none')
}

export interface ResolvedTarget {
  el: Element
  /** inside the sidebar, which is an off-canvas drawer on phones and tablets */
  inDrawer: boolean
}

/**
 * The first matching target of a step. Elements in the off-canvas drawer
 * count as present even while it is closed (`drawerClosed`), because the
 * tour opens the drawer for them; other elements must be visible.
 */
export function resolveTarget(
  step: TourStep,
  root: ParentNode,
  opts: { drawerClosed?: boolean; isVisible?: (el: Element) => boolean } = {},
): ResolvedTarget | null {
  const visible = opts.isVisible ?? defaultVisible
  for (const sel of step.targets) {
    let el: Element | null = null
    try {
      el = root.querySelector(sel)
    } catch {
      continue
    }
    if (!el) continue
    const inDrawer = !!el.closest('#app-sidebar')
    if ((inDrawer && opts.drawerClosed) || visible(el)) return { el, inDrawer }
  }
  return null
}

/** The spotlight hole around a target: padded and kept inside the viewport. */
export function spotlight(target: Box, viewport: { width: number; height: number }, pad = 6): Box {
  const left = Math.max(2, target.left - pad)
  const top = Math.max(2, target.top - pad)
  const right = Math.min(viewport.width - 2, target.left + target.width + pad)
  const bottom = Math.min(viewport.height - 2, target.top + target.height + pad)
  return { left, top, width: Math.max(0, right - left), height: Math.max(0, bottom - top) }
}

export type Placement = 'center' | 'right' | 'bottom' | 'top' | 'left'

/**
 * Where the step card goes: next to the target (right, below, above, left:
 * the first side with room), clamped to the viewport; centred without a
 * target. On narrow screens the card spans the width, above or below the target.
 */
export function placeCard(
  target: Box | null,
  card: { width: number; height: number },
  viewport: { width: number; height: number },
  opts: { gap?: number; margin?: number; narrow?: number } = {},
): { top: number; left: number; placement: Placement } {
  const gap = opts.gap ?? 14
  const m = opts.margin ?? 12
  const clampX = (x: number) => Math.max(m, Math.min(x, viewport.width - card.width - m))
  const clampY = (y: number) => Math.max(m, Math.min(y, viewport.height - card.height - m))
  const center = { top: clampY((viewport.height - card.height) / 2), left: clampX((viewport.width - card.width) / 2), placement: 'center' as const }
  if (!target) return center
  const below = target.top + target.height + gap
  const above = target.top - gap - card.height
  if (viewport.width <= (opts.narrow ?? 640)) {
    if (below + card.height <= viewport.height - m) return { top: below, left: clampX((viewport.width - card.width) / 2), placement: 'bottom' }
    if (above >= m) return { top: above, left: clampX((viewport.width - card.width) / 2), placement: 'top' }
    return center
  }
  const right = target.left + target.width + gap
  if (right + card.width <= viewport.width - m) return { top: clampY(target.top), left: right, placement: 'right' }
  if (below + card.height <= viewport.height - m) return { top: below, left: clampX(target.left), placement: 'bottom' }
  if (above >= m) return { top: above, left: clampX(target.left), placement: 'top' }
  const left = target.left - gap - card.width
  if (left >= m) return { top: clampY(target.top), left, placement: 'left' }
  return center
}

/** The elements a focus trap cycles through, in DOM order. */
export function focusables(root: ParentNode): HTMLElement[] {
  return Array.from(
    root.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'),
  )
}

/** The element to focus for Tab / Shift+Tab inside a trap, or null to let the browser move focus. */
export function trapFocus(items: readonly HTMLElement[], active: Element | null, shift: boolean): HTMLElement | null {
  if (!items.length) return null
  const first = items[0]
  const last = items[items.length - 1]
  const inside = !!active && items.includes(active as HTMLElement)
  if (shift && (active === first || !inside)) return last
  if (!shift && (active === last || !inside)) return first
  return null
}
