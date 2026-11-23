import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '@/lib/api'
import { createWidgetLoader, refreshDashboards } from './widgetData'

const body = (id: string, type = 'coverage', project_id = 'p1') => ({ id, type, project_id, settings: {} })

describe('widget loader', () => {
  afterEach(() => vi.restoreAllMocks())

  it('batches the requests of one moment and caches the answers', async () => {
    const post = vi.spyOn(api, 'post').mockImplementation(async (_path, b) => {
      const ws = (b as { widgets: { id: string }[] }).widgets
      return { results: Object.fromEntries(ws.map((w) => [w.id, w.id === 'b' ? { data: null, error: { status: 403, code: 'forbidden', error: 'no' } } : { data: { n: w.id } }])) } as never
    })
    const l = createWidgetLoader(1)
    l.request('a', body('a'))
    l.request('b', body('b'))
    l.request('a', body('a'))
    expect(l.entries.get('a')?.loading).toBe(true)
    await l.flush()
    expect(post).toHaveBeenCalledTimes(1)
    expect((post.mock.calls[0][1] as { widgets: unknown[] }).widgets).toHaveLength(2)
    expect(l.entries.get('a')).toMatchObject({ data: { n: 'a' }, error: null, loaded: true, loading: false })
    expect(l.entries.get('b')?.error?.status).toBe(403)
    // cached: no new request
    l.request('a', body('a'))
    await l.flush()
    expect(post).toHaveBeenCalledTimes(1)
    l.dispose()
  })

  it('refreshes by type or project, keeping the old data meanwhile', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ results: { a: { data: 1 }, c: { data: 2 } } } as never)
    const l = createWidgetLoader(1)
    l.request('a', body('a'))
    l.request('c', body('c', 'flaky_tests', 'p2'))
    await l.flush()
    const tick = l.tick.value
    refreshDashboards({ types: ['flaky_tests'] })
    expect(l.tick.value).toBe(tick + 1)
    expect(l.entries.get('c')).toMatchObject({ data: 2, loading: true })
    expect(l.entries.get('a')?.loading).toBe(false)
    await l.flush()
    expect((post.mock.calls[1][1] as { widgets: { id: string }[] }).widgets.map((w) => w.id)).toEqual(['c'])
    l.refresh({ projectId: 'p1' })
    await l.flush()
    expect((post.mock.calls[2][1] as { widgets: { id: string }[] }).widgets.map((w) => w.id)).toEqual(['a'])
    l.refresh()
    await l.flush()
    expect(post).toHaveBeenCalledTimes(4)
    l.dispose()
    refreshDashboards()
    expect(post).toHaveBeenCalledTimes(4)
  })

  it('splits large batches and reports a failed request on every widget', async () => {
    const post = vi.spyOn(api, 'post').mockRejectedValue(new ApiError(500, 'internal', 'boom'))
    const l = createWidgetLoader(1)
    for (let i = 0; i < 61; i++) l.request(`k${i}`, body(`k${i}`))
    await l.flush()
    expect(post).toHaveBeenCalledTimes(2)
    expect(l.entries.get('k60')?.error).toMatchObject({ status: 500, error: 'boom' })
    l.dispose()
  })
})
