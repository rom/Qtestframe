import { describe, expect, it } from 'vitest'
import {
  browserTimeZone,
  createLinkedTestDefault,
  FALLBACK_TIMEZONES,
  lastProjectKey,
  localThemeEntries,
  normalizePrefs,
  pickLocalTheme,
  pageSize,
  preferencesPatch,
  readLastProject,
  resolveDark,
  resolveTheme,
  rowsOptions,
  THEMES,
  startRoute,
  timeZones,
  writeLastProject,
} from './prefs'

function memStore() {
  const m = new Map<string, string>()
  return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), m }
}

describe('normalizePrefs', () => {
  it('keeps valid values and drops invalid ones', () => {
    expect(normalizePrefs(null)).toEqual({})
    expect(
      normalizePrefs({
        theme: 'dark',
        start_page: 'search',
        default_project_id: 'p1',
        default_tenant_id: 't1',
        page_size: 50,
        date_format: 'relative',
        timezone: 'Europe/Stockholm',
        compact_tables: true,
        hide_analysis: true,
        create_linked_test: false,
        autocomplete: 'history',
      }),
    ).toEqual({
      theme: 'dark',
      start_page: 'search',
      default_project_id: 'p1',
      default_tenant_id: 't1',
      page_size: 50,
      date_format: 'relative',
      timezone: 'Europe/Stockholm',
      compact_tables: true,
      hide_analysis: true,
      create_linked_test: false,
      autocomplete: 'history',
    })
    // obligation keyword highlighting: on unless turned off
    expect(normalizePrefs({ hide_obligation_highlight: true })).toEqual({ hide_obligation_highlight: true })
    expect(normalizePrefs({ hide_obligation_highlight: false })).toEqual({})
    expect(normalizePrefs({ theme: 'pink', start_page: 'x', page_size: 7, date_format: 'y', compact_tables: false, create_linked_test: null, autocomplete: 'sometimes' })).toEqual({})
  })
})

describe('preferencesPatch', () => {
  it('sends null for defaults', () => {
    expect(preferencesPatch({ theme: 'dark', page_size: 0, timezone: '', compact_tables: false, hide_analysis: true, create_linked_test: false })).toEqual({
      theme: 'dark',
      start_page: null,
      default_project_id: null,
      default_tenant_id: null,
      page_size: null,
      date_format: null,
      timezone: null,
      compact_tables: null,
      hide_analysis: true,
      create_linked_test: false,
      hide_obligation_highlight: null,
      autocomplete: null,
    })
    expect(preferencesPatch({ create_linked_test: null }).create_linked_test).toBeNull()
    expect(preferencesPatch({ autocomplete: 'off' }).autocomplete).toBe('off')
    expect(preferencesPatch({ page_size: 25 }).page_size).toBe(25)
    expect(preferencesPatch({ hide_obligation_highlight: true }).hide_obligation_highlight).toBe(true)
    expect(preferencesPatch({ hide_obligation_highlight: false }).hide_obligation_highlight).toBeNull()
  })
})

describe('resolveDark', () => {
  it('prefers the preference, then local, then OS', () => {
    expect(resolveDark('dark', 'light', false)).toBe(true)
    expect(resolveDark('light', 'dark', true)).toBe(false)
    expect(resolveDark('system', 'light', true)).toBe(true)
    expect(resolveDark('system', 'dark', false)).toBe(false)
    expect(resolveDark('', 'dark', false)).toBe(true)
    expect(resolveDark(undefined, 'light', true)).toBe(false)
    expect(resolveDark(null, null, true)).toBe(true)
    expect(resolveDark(null, 'junk', false)).toBe(false)
  })
})

describe('decorative themes', () => {
  it('accepts every theme value', () => {
    expect(THEMES).toHaveLength(22)
    for (const t of ['spring', 'autumn', 'desert', 'camouflage', 'city_camouflage', 'arctic_camouflage', 'navy', 'midnight', 'green_console', 'amber_terminal', 'crazy', 'ocean', 'forest', 'sunset', 'nordic', 'lavender', 'rose', 'high_contrast', 'high_contrast_light']) {
      expect(normalizePrefs({ theme: t }).theme).toBe(t)
    }
  })
  it('resolves the painted theme', () => {
    expect(resolveTheme('navy', 'light', false)).toBe('navy')
    expect(resolveTheme('system', 'navy', true)).toBe('dark')
    expect(resolveTheme('system', null, false)).toBe('light')
    expect(resolveTheme('', 'crazy', true)).toBe('crazy')
    expect(resolveTheme(undefined, 'system', true)).toBe('dark')
    expect(resolveTheme('pink', 'junk', true)).toBe('dark')
    expect(resolveTheme(null, null, false)).toBe('light')
  })
  it('knows the scheme of decorative themes', () => {
    expect(resolveDark('navy', null, false)).toBe(true)
    expect(resolveDark('green_console', null, false)).toBe(true)
    expect(resolveDark('spring', null, true)).toBe(false)
    expect(resolveDark(null, 'camouflage', false)).toBe(true)
  })
  it('picks the local theme', () => {
    expect(pickLocalTheme('navy', 'dark')).toBe('navy')
    expect(pickLocalTheme('light', 'dark')).toBe('dark')
    expect(pickLocalTheme(null, 'light')).toBe('light')
    expect(pickLocalTheme('junk', 'junk')).toBeNull()
    expect(pickLocalTheme(undefined, undefined)).toBeNull()
  })
  it('says what to remember locally', () => {
    expect(localThemeEntries('system')).toEqual({ scheme: null, decor: null })
    expect(localThemeEntries('light')).toEqual({ scheme: 'light', decor: null })
    expect(localThemeEntries('dark')).toEqual({ scheme: 'dark', decor: null })
    expect(localThemeEntries('navy')).toEqual({ scheme: 'dark', decor: 'navy' })
    expect(localThemeEntries('crazy')).toEqual({ scheme: 'light', decor: 'crazy' })
  })
})

describe('page sizes', () => {
  it('uses the preference or the fallback', () => {
    expect(pageSize({ page_size: 10 })).toBe(10)
    expect(pageSize({ page_size: 11 })).toBe(25)
    expect(pageSize(null, 50)).toBe(50)
  })
  it('builds rows options', () => {
    expect(rowsOptions(25)).toEqual([10, 25, 50, 100, 250])
    expect(rowsOptions(10, [25, 50])).toEqual([10, 25, 50])
    expect(rowsOptions(0, [50])).toEqual([50])
  })
})

describe('createLinkedTestDefault', () => {
  it('overrides the server default when set', () => {
    expect(createLinkedTestDefault({ create_linked_test: false }, true)).toBe(false)
    expect(createLinkedTestDefault({ create_linked_test: true }, false)).toBe(true)
    expect(createLinkedTestDefault({}, false)).toBe(false)
    expect(createLinkedTestDefault(null, undefined)).toBe(true)
  })
})

describe('startRoute', () => {
  it('routes by start page', () => {
    expect(startRoute(null)).toEqual({ name: 'projects' })
    expect(startRoute({ start_page: 'projects' })).toEqual({ name: 'projects' })
    expect(startRoute({ start_page: 'search' })).toEqual({ name: 'search' })
    expect(startRoute({ start_page: 'last_project' }, { lastProjectId: 'p1' })).toEqual({ name: 'dashboard', params: { projectId: 'p1' } })
    expect(startRoute({ start_page: 'last_project' }, {})).toEqual({ name: 'projects' })
    expect(startRoute({ start_page: 'last_project' }, { lastProjectId: 'gone', projectIds: ['p1'] })).toEqual({ name: 'projects' })
    expect(startRoute({ start_page: 'default_project', default_project_id: 'p2' }, { projectIds: ['p2'] })).toEqual({ name: 'dashboard', params: { projectId: 'p2' } })
    expect(startRoute({ start_page: 'default_project' })).toEqual({ name: 'projects' })
  })
})

describe('last project', () => {
  it('reads and writes per tenant', () => {
    const s = memStore()
    expect(lastProjectKey('t1')).toBe('qtf.lastProject.t1')
    expect(lastProjectKey(null)).toBe('qtf.lastProject')
    expect(readLastProject('t1', s)).toBeNull()
    writeLastProject('t1', 'p9', s)
    expect(readLastProject('t1', s)).toBe('p9')
    expect(readLastProject('t2', s)).toBeNull()
  })
  it('survives broken storage', () => {
    const bad = {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('denied')
      },
    }
    expect(readLastProject('t', bad)).toBeNull()
    expect(() => writeLastProject('t', 'p', bad)).not.toThrow()
    expect(readLastProject('t', null)).toBeNull()
    writeLastProject('t', 'p', null)
  })
  it('uses localStorage by default', () => {
    writeLastProject('tx', 'px')
    expect(readLastProject('tx')).toBe('px')
  })
})

describe('timeZones', () => {
  it('uses Intl.supportedValuesOf when available', () => {
    const z = timeZones('Mars/Base', { supportedValuesOf: () => ['Europe/Berlin', 'Asia/Tokyo'] })
    expect(z).toEqual(['UTC', 'Asia/Tokyo', 'Europe/Berlin', 'Mars/Base'])
  })
  it('falls back when missing or failing', () => {
    expect(timeZones(null, {})).toEqual(expect.arrayContaining(FALLBACK_TIMEZONES))
    expect(
      timeZones(undefined, {
        supportedValuesOf: () => {
          throw new Error('x')
        },
      })[0],
    ).toBe('UTC')
    expect(timeZones(undefined, { supportedValuesOf: () => [] }).length).toBe(FALLBACK_TIMEZONES.length)
    expect(timeZones().length).toBeGreaterThan(10)
  })
  it('knows the browser zone', () => {
    expect(typeof browserTimeZone()).toBe('string')
  })
})
