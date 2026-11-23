// The dashboard widget registry of the client (the server has the matching
// one with the data providers: internal/api/dashboardwidgets.go). A widget
// type has a default size, the settings it takes (shown in the settings
// dialog) and the permissions it needs to be offered in "Add widget" (its
// data is checked by the server either way).
import { P } from './permissions'
import { freeSpot, type GridItem } from './dashboardGrid'

export type SettingKind = 'select' | 'number' | 'text' | 'query' | 'markdown' | 'release' | 'cycle' | 'variant' | 'project'

export interface SettingField {
  key: string
  kind: SettingKind
  /** select: the values (labels: dashboards.opt.<key>.<value>) */
  options?: readonly string[]
  default?: string | number
  min?: number
  max?: number
}

export interface WidgetDef {
  type: string
  icon: string
  w: number
  h: number
  /** about one project (on the home dashboard settings.project_id names it) */
  project: boolean
  /** the permissions to offer it (all of them) */
  perms?: readonly string[]
  fields: readonly SettingField[]
  /** the widget loads its own data (it has its own controls) */
  selfLoading?: boolean
}

export const CHART_TYPES = ['bar', 'hbar', 'doughnut', 'table'] as const
export type ChartKind = (typeof CHART_TYPES)[number]

const chart = (options: readonly ChartKind[], def: ChartKind): SettingField => ({ key: 'chart', kind: 'select', options, default: def })
const limit = (def: number, max = 50): SettingField => ({ key: 'limit', kind: 'number', default: def, min: 1, max })
const range: SettingField = { key: 'range', kind: 'select', options: ['30', '90', '365'], default: '30' }

/** The dimensions of the breakdown widget (server: breakdownDims). */
export const BREAKDOWN_DIMENSIONS = [
  'requirement_status',
  'requirement_obligation',
  'review_state',
  'test_status',
  'test_execution_type',
  'run_status',
  'defect_severity',
  'defect_status',
  'risk_level',
  'risk_status',
  'risk_mitigation',
] as const

/** The item kinds of the saved query widget (server: savedQueryEntities). */
export const QUERY_ENTITIES = ['requirements', 'tests', 'defects', 'risks', 'runs'] as const

const defs: WidgetDef[] = [
  { type: 'key_figures', icon: 'pi pi-th-large', w: 12, h: 3, project: true, fields: [] },
  { type: 'coverage', icon: 'pi pi-shield', w: 6, h: 4, project: true, fields: [chart(['doughnut', 'bar', 'table'], 'doughnut')] },
  { type: 'execution_results', icon: 'pi pi-chart-pie', w: 6, h: 4, project: true, fields: [chart(['doughnut', 'bar', 'table'], 'doughnut')] },
  { type: 'verification_status', icon: 'pi pi-verified', w: 6, h: 4, project: true, perms: [P.requirementRead], fields: [chart(['doughnut', 'bar', 'table'], 'doughnut')] },
  {
    type: 'breakdown',
    icon: 'pi pi-chart-bar',
    w: 6,
    h: 4,
    project: true,
    fields: [{ key: 'dimension', kind: 'select', options: BREAKDOWN_DIMENSIONS, default: 'requirement_status' }, chart(CHART_TYPES, 'bar')],
  },
  { type: 'test_types', icon: 'pi pi-check-square', w: 6, h: 4, project: true, fields: [chart(['bar', 'table'], 'bar')] },
  { type: 'active_runs', icon: 'pi pi-play-circle', w: 6, h: 4, project: true, fields: [] },
  { type: 'recent_runs', icon: 'pi pi-history', w: 6, h: 4, project: true, fields: [] },
  { type: 'test_results_trend', icon: 'pi pi-chart-line', w: 12, h: 5, project: true, fields: [range] },
  { type: 'defects_per_week', icon: 'pi pi-calendar', w: 6, h: 4, project: true, fields: [range] },
  { type: 'recent_activity', icon: 'pi pi-wave-pulse', w: 12, h: 5, project: true, fields: [] },
  { type: 'flaky_tests', icon: 'pi pi-sync', w: 12, h: 5, project: true, fields: [limit(10, 10)] },
  { type: 'coverage_by_group', icon: 'pi pi-folder', w: 6, h: 4, project: true, fields: [{ key: 'group', kind: 'select', options: ['folder', 'category'], default: 'folder' }] },
  { type: 'defect_age', icon: 'pi pi-clock', w: 6, h: 4, project: true, fields: [] },
  { type: 'risk_heat', icon: 'pi pi-table', w: 6, h: 4, project: true, fields: [] },
  { type: 'trace_rules', icon: 'pi pi-sitemap', w: 6, h: 4, project: true, fields: [] },
  { type: 'traceability_gaps', icon: 'pi pi-link', w: 6, h: 4, project: true, perms: [P.requirementRead, P.testRead], fields: [limit(5)] },
  { type: 'correlations', icon: 'pi pi-ban', w: 12, h: 2, project: true, fields: [] },
  { type: 'release_progress', icon: 'pi pi-flag', w: 6, h: 4, project: true, fields: [] },
  {
    type: 'release_readiness',
    icon: 'pi pi-flag-fill',
    w: 6,
    h: 5,
    project: true,
    fields: [
      { key: 'release_id', kind: 'release' },
      { key: 'variant', kind: 'variant' },
    ],
  },
  { type: 'release_burnup', icon: 'pi pi-chart-line', w: 6, h: 4, project: true, fields: [{ key: 'release_id', kind: 'release' }] },
  { type: 'cycle_burndown', icon: 'pi pi-sort-amount-down', w: 6, h: 4, project: true, perms: [P.runRead], fields: [{ key: 'cycle_id', kind: 'cycle' }] },
  { type: 'quality_metrics', icon: 'pi pi-gauge', w: 12, h: 9, project: true, selfLoading: true, fields: [] },
  { type: 'consistency', icon: 'pi pi-check-circle', w: 12, h: 5, project: true, perms: [P.requirementRead], selfLoading: true, fields: [] },
  { type: 'exploratory_metrics', icon: 'pi pi-compass', w: 6, h: 4, project: true, perms: [P.runRead], fields: [] },
  {
    type: 'framework_coverage',
    icon: 'pi pi-shield',
    w: 6,
    h: 4,
    project: true,
    perms: [P.requirementRead],
    fields: [{ key: 'framework', kind: 'select', options: ['attack', 'd3fend', 'emb3d'], default: 'attack' }],
  },
  {
    type: 'saved_query',
    icon: 'pi pi-filter',
    w: 6,
    h: 4,
    project: true,
    fields: [
      { key: 'entity', kind: 'select', options: QUERY_ENTITIES, default: 'requirements' },
      { key: 'query', kind: 'query', default: '' },
      { key: 'display', kind: 'select', options: ['table', 'count'], default: 'table' },
      limit(10),
    ],
  },
  { type: 'my_work', icon: 'pi pi-inbox', w: 12, h: 3, project: false, fields: [limit(5)] },
  {
    type: 'discussions',
    icon: 'pi pi-comments',
    w: 6,
    h: 5,
    project: false,
    fields: [
      { key: 'status', kind: 'select', options: ['open', 'resolved', 'all'], default: 'open' },
      { key: 'filter', kind: 'select', options: ['', 'mine', 'mentioned'], default: '' },
      limit(8),
    ],
  },
  { type: 'note', icon: 'pi pi-pencil', w: 6, h: 3, project: false, fields: [{ key: 'text', kind: 'markdown', default: '' }] },
]

export const WIDGETS: Readonly<Record<string, WidgetDef>> = Object.fromEntries(defs.map((d) => [d.type, d]))
export const WIDGET_TYPES: readonly string[] = defs.map((d) => d.type)

/** A widget of a dashboard (model.DashboardWidget). */
export interface DashboardWidget extends GridItem {
  type: string
  title?: string
  settings?: Record<string, unknown>
}

/** The settings fields of a widget type on a dashboard (home: with the project). */
export function settingFields(type: string, home: boolean): SettingField[] {
  const d = WIDGETS[type]
  if (!d) return []
  return home && d.project ? [{ key: 'project_id', kind: 'project' }, ...d.fields] : [...d.fields]
}

/** A setting of a widget, or the field's default. */
export function setting<T = unknown>(w: Pick<DashboardWidget, 'type' | 'settings'>, key: string): T {
  const v = w.settings?.[key]
  if (v !== undefined && v !== null && v !== '') return v as T
  const f = WIDGETS[w.type]?.fields.find((x) => x.key === key)
  return (f?.default ?? '') as T
}

/** The widget types the caller may add (home: the home dashboard). */
export function availableWidgets(can: (perms: readonly string[]) => boolean): WidgetDef[] {
  return defs.filter((d) => !d.perms || can(d.perms))
}

let seq = 0
/** A new widget id, unique in the layout. */
export function newWidgetId(items: readonly { id: string }[]): string {
  const ids = new Set(items.map((i) => i.id))
  let id: string
  do {
    seq++
    id = `w${Date.now().toString(36)}${seq.toString(36)}`
  } while (ids.has(id))
  return id
}

/** A new widget of a type in the first free place of the layout. */
export function newWidget(type: string, items: readonly DashboardWidget[], settings: Record<string, unknown> = {}): DashboardWidget {
  const d = WIDGETS[type]
  const w = d?.w ?? 6
  const h = d?.h ?? 4
  const { x, y } = freeSpot(items, w, h)
  const s: Record<string, unknown> = { ...settings }
  return { id: newWidgetId(items), type, x, y, w, h, ...(Object.keys(s).length ? { settings: s } : {}) }
}

/** Settings without empty values (what is saved). */
export function cleanSettings(s: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  if (!s) return undefined
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(s)) if (v !== undefined && v !== null && v !== '') out[k] = v
  return Object.keys(out).length ? out : undefined
}

/**
 * The request of a widget for POST /dashboards/data and its cache key:
 * the type, the project and the settings that change its data (the title,
 * the chart type and the display do not).
 */
const VIEW_ONLY = new Set(['chart', 'display', 'text'])
export function dataRequest(w: DashboardWidget, projectId: string | undefined, overrides: Record<string, unknown> = {}) {
  const settings: Record<string, unknown> = {}
  for (const [k, v] of Object.entries({ ...(w.settings ?? {}), ...overrides })) {
    if (!VIEW_ONLY.has(k) && v !== undefined && v !== null && v !== '') settings[k] = v
  }
  const project = projectId || (settings.project_id as string) || ''
  const key = JSON.stringify([w.type, project, Object.entries(settings).sort(([a], [b]) => a.localeCompare(b))])
  return { key, body: { id: key, type: w.type, project_id: project || undefined, settings } }
}

/** Whether a widget asks the server for data. */
export function loadsData(type: string): boolean {
  const d = WIDGETS[type]
  return !!d && !d.selfLoading && type !== 'note'
}
