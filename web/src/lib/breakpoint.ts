// Media-query state for the few places where layout needs logic (which
// component to render), not only CSS. Breakpoints match styles.css.
import { getCurrentScope, onScopeDispose, readonly, ref, type Ref } from 'vue'

/** Upper bounds (px, inclusive) of the layout breakpoints. */
export const BREAKPOINTS = {
  /** phones: stacked tables, single-column forms, full-screen drawers */
  phone: 640,
  /** below this the sidebar is off-canvas */
  tablet: 1023,
  /** below this the folder tree of the requirement/test workspace is a drawer */
  wide: 1199,
} as const

export type BreakpointName = keyof typeof BREAKPOINTS

type MatchMedia = (query: string) => Pick<MediaQueryList, 'matches'> & Partial<Pick<MediaQueryList, 'addEventListener' | 'removeEventListener'>>

/** "(max-width: 640px)" for a breakpoint name or a pixel width. */
export function maxWidthQuery(bp: BreakpointName | number): string {
  const px = typeof bp === 'number' ? bp : BREAKPOINTS[bp]
  return `(max-width: ${px}px)`
}

function defaultMatchMedia(): MatchMedia | null {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' ? window.matchMedia.bind(window) : null
}

/**
 * Reactive `matches` of a media query. Without matchMedia (tests, SSR) it
 * is `fallback`. The listener is removed when the calling scope ends.
 */
export function useMediaQuery(query: string, fallback = false, mm: MatchMedia | null = defaultMatchMedia()): Readonly<Ref<boolean>> {
  const matches = ref(fallback)
  if (!mm) return readonly(matches)
  let mql: ReturnType<MatchMedia>
  try {
    mql = mm(query)
  } catch {
    return readonly(matches)
  }
  matches.value = !!mql.matches
  const onChange = (e: Event) => {
    matches.value = !!(e as MediaQueryListEvent).matches
  }
  mql.addEventListener?.('change', onChange)
  if (getCurrentScope()) onScopeDispose(() => mql.removeEventListener?.('change', onChange))
  return readonly(matches)
}

/** True at or below a breakpoint (e.g. useBreakpoint('phone')). */
export function useBreakpoint(bp: BreakpointName | number, mm: MatchMedia | null = defaultMatchMedia()): Readonly<Ref<boolean>> {
  return useMediaQuery(maxWidthQuery(bp), false, mm)
}
