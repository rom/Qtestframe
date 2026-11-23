// Dashboards (model.Dashboard): layouts of widgets, their export as JSON
// and the checks of an import (the server checks again).
import { GRID_COLS, MAX_H, MAX_ROWS, resolve } from './dashboardGrid'
import { WIDGETS, cleanSettings, newWidgetId, type DashboardWidget } from './widgets'

export interface Dashboard {
  id: string
  project_id?: string
  owner_id?: string
  name: string
  /** the built-in layout it replaces (shared) or was copied from */
  builtin?: string
  source_id?: string
  widgets: DashboardWidget[]
  shared: boolean
  /** false: a built-in layout nobody changed yet */
  stored: boolean
  default: boolean
  editable: boolean
}

export interface DashboardList {
  items: Dashboard[]
  default_id: string
  can_manage: boolean
}

export const EXPORT_FORMAT = 'qtestframe-dashboard'
export const MAX_WIDGETS = 60

/** The English names the server gives copies of the built-in layouts. */
const BUILTIN_NAMES: Record<string, string> = { overview: 'Overview', trends: 'Trends', quality: 'Quality', home: 'Home' }

/**
 * The name to show: a built-in layout (or the shared dashboard that
 * replaces it, still with its built-in name) in the user's language.
 */
export function dashboardName(d: Pick<Dashboard, 'name' | 'builtin' | 'shared'>, builtinLabel: (key: string) => string): string {
  if (d.builtin && (d.shared || d.builtin === 'home') && (!d.name || d.name === BUILTIN_NAMES[d.builtin])) return builtinLabel(d.builtin)
  return d.name || (d.builtin ? builtinLabel(d.builtin) : '')
}

/** The JSON export of a dashboard. */
export function exportDashboard(d: Pick<Dashboard, 'name' | 'widgets'>, name = d.name) {
  return {
    format: EXPORT_FORMAT,
    version: 1,
    name,
    widgets: d.widgets.map((w) => {
      const s = cleanSettings(w.settings)
      // a project of the home dashboard means nothing elsewhere
      return { id: w.id, type: w.type, ...(w.title ? { title: w.title } : {}), x: w.x, y: w.y, w: w.w, h: w.h, ...(s ? { settings: s } : {}) }
    }),
  }
}

/** A file name for the export. */
export function exportFileName(name: string): string {
  const base = name
    .normalize('NFKD')
    .replace(/[^\w\s-]/g, '')
    .trim()
    .replace(/\s+/g, '-')
    .toLowerCase()
  return `dashboard-${base || 'export'}.json`
}

export class ImportError extends Error {
  constructor(
    public readonly key: string,
    public readonly params: Record<string, unknown> = {},
  ) {
    super(key)
  }
}

/**
 * Parses an exported dashboard: the format and version, known widget types
 * (others are left out and counted), sizes within the grid, unique ids and
 * no overlaps. Throws ImportError (key: an i18n key under dashboards.import).
 */
export function parseImport(text: string): { name: string; widgets: DashboardWidget[]; skipped: number } {
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    throw new ImportError('notJson')
  }
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new ImportError('notDashboard')
  const o = raw as Record<string, unknown>
  if (o.format !== EXPORT_FORMAT || !Array.isArray(o.widgets)) throw new ImportError('notDashboard')
  if (typeof o.version === 'number' && o.version > 1) throw new ImportError('newer', { version: o.version })
  if (o.widgets.length > MAX_WIDGETS) throw new ImportError('tooMany', { max: MAX_WIDGETS })
  let skipped = 0
  const widgets: DashboardWidget[] = []
  for (const x of o.widgets as unknown[]) {
    if (!x || typeof x !== 'object') {
      skipped++
      continue
    }
    const w = x as Record<string, unknown>
    if (typeof w.type !== 'string' || !WIDGETS[w.type]) {
      skipped++
      continue
    }
    const num = (v: unknown, d: number) => (typeof v === 'number' && Number.isFinite(v) ? Math.round(v) : d)
    const wd = Math.min(GRID_COLS, Math.max(1, num(w.w, WIDGETS[w.type].w)))
    let id = typeof w.id === 'string' && w.id.trim() && w.id.length <= 64 ? w.id.trim() : ''
    if (!id || widgets.some((y) => y.id === id)) id = newWidgetId(widgets)
    const settings = w.settings && typeof w.settings === 'object' && !Array.isArray(w.settings) ? { ...(w.settings as Record<string, unknown>) } : undefined
    widgets.push({
      id,
      type: w.type,
      ...(typeof w.title === 'string' && w.title.trim() ? { title: w.title.trim().slice(0, 200) } : {}),
      x: Math.min(GRID_COLS - wd, Math.max(0, num(w.x, 0))),
      y: Math.min(MAX_ROWS, Math.max(0, num(w.y, 0))),
      w: wd,
      h: Math.min(MAX_H, Math.max(1, num(w.h, WIDGETS[w.type].h))),
      ...(settings ? { settings } : {}),
    })
  }
  const name = typeof o.name === 'string' ? o.name.trim().slice(0, 100) : ''
  return { name, widgets: resolve(widgets), skipped }
}
