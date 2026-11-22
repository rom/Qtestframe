// Widget data of a dashboard: the widgets on screen ask for their data and
// the requests of one moment go to the server together (POST
// /dashboards/data, at most 60 per request); the answers are kept per
// widget request (type, project and data settings) while the dashboard is
// shown. refresh() is the one way to load widgets again: the refresh button
// uses it, and live updates can call refreshDashboards() for every open
// dashboard.
import { onBeforeUnmount, reactive, ref } from 'vue'
import { api, ApiError, errorMessage } from '@/lib/api'
import { MAX_WIDGETS } from '@/lib/dashboards'

export interface WidgetError {
  status: number
  code: string
  error: string
}

export interface WidgetEntry {
  data: unknown
  error: WidgetError | null
  loading: boolean
  loaded: boolean
}

export interface WidgetRequestBody {
  id: string
  type: string
  project_id?: string
  settings: Record<string, unknown>
}

/** What refresh() reloads (nothing given: every widget). */
export interface RefreshFilter {
  types?: readonly string[]
  projectId?: string
}

export interface WidgetLoader {
  entries: Map<string, WidgetEntry>
  /** bumped by refresh(): widgets that load their own data reload on it */
  tick: { value: number }
  request(key: string, body: WidgetRequestBody): void
  refresh(filter?: RefreshFilter): void
  flush(): Promise<void>
  dispose(): void
}

const loaders = new Set<WidgetLoader>()

/** Refreshes the widgets of every dashboard shown (the hook for live updates). */
export function refreshDashboards(filter?: RefreshFilter): void {
  for (const l of loaders) l.refresh(filter)
}

function matches(body: WidgetRequestBody, f?: RefreshFilter): boolean {
  if (!f) return true
  if (f.types && !f.types.includes(body.type)) return false
  if (f.projectId && body.project_id && body.project_id !== f.projectId) return false
  return true
}

/** A loader not tied to a component (tests); see useWidgetLoader. */
export function createWidgetLoader(delay = 15): WidgetLoader {
  const entries = reactive(new Map<string, WidgetEntry>()) as Map<string, WidgetEntry>
  const bodies = new Map<string, WidgetRequestBody>()
  const pending = new Map<string, WidgetRequestBody>()
  const tick = ref(0)
  let timer: ReturnType<typeof setTimeout> | null = null
  let flushing: Promise<void> = Promise.resolve()

  function schedule() {
    if (timer) return
    timer = setTimeout(() => {
      timer = null
      void flush()
    }, delay)
  }

  async function send(batch: WidgetRequestBody[]) {
    try {
      const res = await api.post<{ results: Record<string, { data: unknown; error?: WidgetError }> }>('/dashboards/data', { widgets: batch })
      for (const b of batch) {
        const r = res.results?.[b.id]
        entries.set(b.id, { data: r?.data ?? null, error: r?.error ?? (r ? null : { status: 0, code: 'missing', error: '' }), loading: false, loaded: true })
      }
    } catch (e) {
      const err: WidgetError = { status: e instanceof ApiError ? e.status : 0, code: e instanceof ApiError ? e.code : 'network', error: errorMessage(e) }
      for (const b of batch) entries.set(b.id, { data: entries.get(b.id)?.data ?? null, error: err, loading: false, loaded: true })
    }
  }

  async function flush() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    const all = [...pending.values()]
    pending.clear()
    const jobs: Promise<void>[] = []
    for (let i = 0; i < all.length; i += MAX_WIDGETS) jobs.push(send(all.slice(i, i + MAX_WIDGETS)))
    flushing = Promise.all([flushing, ...jobs]).then(() => undefined)
    await flushing
  }

  const loader: WidgetLoader = {
    entries,
    tick,
    request(key, body) {
      const e = entries.get(key)
      if (e && (e.loading || e.loaded)) return
      bodies.set(key, body)
      entries.set(key, { data: e?.data ?? null, error: null, loading: true, loaded: false })
      pending.set(key, body)
      schedule()
    },
    refresh(filter) {
      tick.value++
      for (const [key, body] of bodies) {
        if (!matches(body, filter)) continue
        const e = entries.get(key)
        // the old data stays on screen until the new arrives
        entries.set(key, { data: e?.data ?? null, error: null, loading: true, loaded: false })
        pending.set(key, body)
      }
      if (pending.size) schedule()
    },
    flush,
    dispose() {
      if (timer) clearTimeout(timer)
      loaders.delete(loader)
    },
  }
  loaders.add(loader)
  return loader
}

/** The widget loader of a dashboard component (disposed with it). */
export function useWidgetLoader(): WidgetLoader {
  const l = createWidgetLoader()
  onBeforeUnmount(() => l.dispose())
  return l
}
