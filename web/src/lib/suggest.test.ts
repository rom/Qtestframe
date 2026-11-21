import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  autocompleteEnabled,
  autocompleteMode,
  canSuggest,
  clearHistory,
  clearSuggestCache,
  debounce,
  fetchSuggestions,
  historyCount,
  isSuggestField,
  nextActive,
  queryOf,
  recordsHistory,
  rememberValues,
  savedEntries,
  CACHE_MS,
  MAX_QUERY,
  MAX_VALUE,
  SUGGEST_FIELDS,
} from './suggest'

function res(status: number, body?: unknown): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}
const fetchMock = vi.fn()
beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  clearSuggestCache()
})
afterEach(() => vi.unstubAllGlobals())

describe('preference', () => {
  it('maps the preference to sources', () => {
    expect(autocompleteMode(null)).toBe('')
    expect(autocompleteMode({ autocomplete: 'weird' })).toBe('')
    expect(autocompleteMode({ autocomplete: 'project' })).toBe('project')
    expect(autocompleteEnabled({})).toBe(true)
    expect(autocompleteEnabled({ autocomplete: 'history' })).toBe(true)
    expect(autocompleteEnabled({ autocomplete: 'off' })).toBe(false)
    expect(recordsHistory({})).toBe(true)
    expect(recordsHistory({ autocomplete: 'history' })).toBe(true)
    expect(recordsHistory({ autocomplete: 'project' })).toBe(false)
    expect(recordsHistory({ autocomplete: 'off' })).toBe(false)
  })
})

describe('fields and queries', () => {
  it('knows the field keys and the project rule', () => {
    expect(SUGGEST_FIELDS).toContain('test.step.expected')
    expect(isSuggestField('test.step.expected')).toBe(true)
    expect(isSuggestField('password')).toBe(false)
    expect(isSuggestField(undefined)).toBe(false)
    expect(canSuggest('test.title', 'p1')).toBe(true)
    expect(canSuggest('test.title', '')).toBe(false)
    expect(canSuggest('search', '')).toBe(true)
    expect(canSuggest('nope', 'p1')).toBe(false)
  })
  it('asks about the trimmed text', () => {
    expect(queryOf('  abc ')).toBe('abc')
    expect(queryOf('   ')).toBeNull()
    expect(queryOf(undefined)).toBeNull()
    expect(queryOf('', { allowEmpty: true })).toBe('')
    expect(queryOf('x'.repeat(MAX_QUERY + 1))).toBeNull()
  })
  it('moves the active option through "none"', () => {
    expect(nextActive(-1, 3, 1)).toBe(0)
    expect(nextActive(2, 3, 1)).toBe(-1)
    expect(nextActive(-1, 3, -1)).toBe(2)
    expect(nextActive(0, 3, -1)).toBe(-1)
    expect(nextActive(1, 3, 1)).toBe(2)
    expect(nextActive(0, 0, 1)).toBe(-1)
  })
})

describe('debounce', () => {
  it('runs once after the pause and can be cancelled', () => {
    vi.useFakeTimers()
    const fn = vi.fn()
    const d = debounce(fn, 200)
    d.call(1)
    vi.advanceTimersByTime(150)
    d.call(2)
    expect(d.pending()).toBe(true)
    vi.advanceTimersByTime(150)
    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(60)
    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith(2)
    expect(d.pending()).toBe(false)
    d.call(3)
    d.cancel()
    vi.advanceTimersByTime(500)
    expect(fn).toHaveBeenCalledTimes(1)
    vi.useRealTimers()
  })
})

describe('requests', () => {
  it('fetches, caches per field, project and text, and expires', async () => {
    fetchMock.mockImplementation(async () => res(200, { items: [{ value: 'Login works', source: 'history', uses: 2 }], total: 1 }))
    const a = await fetchSuggestions('requirement.title', 'Log', 'p1', { now: 1000 })
    expect(a.map((s) => s.value)).toEqual(['Login works'])
    const url = new URL(fetchMock.mock.calls[0][0] as string, 'http://x')
    expect(url.pathname).toBe('/api/v1/suggest')
    expect(Object.fromEntries(url.searchParams)).toEqual({ field: 'requirement.title', q: 'Log', project_id: 'p1', limit: '8' })
    // same text in another case: cached
    await fetchSuggestions('requirement.title', 'log', 'p1', { now: 2000 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    // another project, or later: asked again
    await fetchSuggestions('requirement.title', 'log', 'p2', { now: 2000 })
    await fetchSuggestions('requirement.title', 'log', 'p1', { now: 1000 + CACHE_MS + 1 })
    expect(fetchMock).toHaveBeenCalledTimes(3)
    // the search box sends no project
    await fetchSuggestions('search', 'st', 'p1')
    expect(new URL(fetchMock.mock.calls[3][0] as string, 'http://x').searchParams.has('project_id')).toBe(false)
  })

  it('records only new values, and only when the preference records', async () => {
    expect(
      savedEntries(
        { 'test.title': ' Login ', 'test.step.action': ['Open', 'Open', '', 'Click', null], 'test.expected': 'x'.repeat(MAX_VALUE + 1) },
        { 'test.title': 'Login', 'test.step.action': ['Click'] },
      ),
    ).toEqual([{ field: 'test.step.action', value: 'Open' }])
    expect(savedEntries({ comment: 'Looks good' })).toEqual([{ field: 'comment', value: 'Looks good' }])

    fetchMock.mockImplementation(async () => res(200, { recorded: 1 }))
    await rememberValues({}, 'p1', [])
    await rememberValues({ autocomplete: 'off' }, 'p1', [{ field: 'comment', value: 'x' }])
    await rememberValues({ autocomplete: 'project' }, 'p1', [{ field: 'comment', value: 'x' }])
    expect(fetchMock).not.toHaveBeenCalled()
    await rememberValues({ autocomplete: 'history' }, 'p1', [{ field: 'comment', value: 'x' }])
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/suggest/history')
    expect(JSON.parse(init.body)).toEqual({ project_id: 'p1', entries: [{ field: 'comment', value: 'x' }] })
    // a failure never reaches the form
    fetchMock.mockRejectedValueOnce(new Error('offline'))
    await expect(rememberValues({}, 'p1', [{ field: 'comment', value: 'y' }])).resolves.toBeUndefined()
  })

  it('counts and clears the history', async () => {
    fetchMock.mockResolvedValueOnce(res(200, { count: 5 }))
    expect(await historyCount()).toBe(5)
    fetchMock.mockResolvedValueOnce(res(200, { deleted: 5 }))
    expect(await clearHistory()).toBe(5)
    expect(fetchMock.mock.calls[1][1].method).toBe('DELETE')
    fetchMock.mockResolvedValueOnce(res(200, {}))
    expect(await clearHistory()).toBe(0)
    fetchMock.mockResolvedValueOnce(res(200, {}))
    expect(await historyCount()).toBe(0)
  })

  it('bounds the cache', async () => {
    fetchMock.mockImplementation(async () => res(200, { items: [] }))
    for (let i = 0; i < 205; i++) await fetchSuggestions('comment', `q${i}`, 'p1', { now: 1 })
    expect(fetchMock).toHaveBeenCalledTimes(205)
    // q0 fell out with the clear at 200 entries
    await fetchSuggestions('comment', 'q0', 'p1', { now: 1 })
    expect(fetchMock).toHaveBeenCalledTimes(206)
  })
})
