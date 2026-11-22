import { describe, expect, it } from 'vitest'
import { ImportError, dashboardName, exportDashboard, exportFileName, parseImport } from './dashboards'
import { WIDGETS, WIDGET_TYPES, availableWidgets, cleanSettings, dataRequest, loadsData, newWidget, setting, settingFields } from './widgets'
import { overlaps } from './dashboardGrid'

const label = (k: string) => `«${k}»`

describe('dashboards', () => {
  it('names built-in layouts in the user language', () => {
    expect(dashboardName({ name: '', builtin: 'overview', shared: true }, label)).toBe('«overview»')
    expect(dashboardName({ name: 'Overview', builtin: 'overview', shared: true }, label)).toBe('«overview»')
    expect(dashboardName({ name: 'Ours', builtin: 'overview', shared: true }, label)).toBe('Ours')
    expect(dashboardName({ name: 'Overview', builtin: 'overview', shared: false }, label)).toBe('Overview')
    expect(dashboardName({ name: '', builtin: 'trends', shared: false }, label)).toBe('«trends»')
    expect(dashboardName({ name: 'Home', builtin: 'home', shared: false }, label)).toBe('«home»')
    expect(dashboardName({ name: 'Mine', shared: false }, label)).toBe('Mine')
  })

  it('exports and imports a dashboard', () => {
    const ex = exportDashboard({
      name: 'Release health',
      widgets: [
        { id: 'a', type: 'saved_query', x: 0, y: 0, w: 6, h: 4, title: 'Open', settings: { query: 'status:draft', entity: '' } },
        { id: 'b', type: 'note', x: 6, y: 0, w: 6, h: 3 },
      ],
    })
    expect(ex.format).toBe('qtestframe-dashboard')
    expect(ex.widgets[0]).toEqual({ id: 'a', type: 'saved_query', title: 'Open', x: 0, y: 0, w: 6, h: 4, settings: { query: 'status:draft' } })
    expect(ex.widgets[1]).toEqual({ id: 'b', type: 'note', x: 6, y: 0, w: 6, h: 3 })
    const back = parseImport(JSON.stringify(ex))
    expect(back.name).toBe('Release health')
    expect(back.skipped).toBe(0)
    expect(back.widgets).toHaveLength(2)
    expect(exportFileName('Release health – Q1!')).toBe('dashboard-release-health-q1.json')
    expect(exportFileName('!!!')).toBe('dashboard-export.json')
  })

  it('checks an import', () => {
    const bad = (text: string, key: string) => {
      try {
        parseImport(text)
      } catch (e) {
        expect(e).toBeInstanceOf(ImportError)
        expect((e as ImportError).key).toBe(key)
        return
      }
      throw new Error('accepted ' + text)
    }
    bad('{', 'notJson')
    bad('[]', 'notDashboard')
    bad('{"format":"other","widgets":[]}', 'notDashboard')
    bad('{"format":"qtestframe-dashboard"}', 'notDashboard')
    bad('{"format":"qtestframe-dashboard","version":2,"widgets":[]}', 'newer')
    bad(JSON.stringify({ format: 'qtestframe-dashboard', widgets: Array.from({ length: 61 }, () => ({ type: 'note' })) }), 'tooMany')
    const r = parseImport(
      JSON.stringify({
        format: 'qtestframe-dashboard',
        widgets: [{ type: 'nope' }, 3, { id: 'x', type: 'note', x: 11, y: -1, w: 40, h: 99 }, { id: 'x', type: 'coverage', x: 0, y: 0 }, { type: 'coverage', title: ' T ', settings: [1] }],
      }),
    )
    expect(r.skipped).toBe(2)
    expect(r.name).toBe('')
    expect(r.widgets).toHaveLength(3)
    expect(new Set(r.widgets.map((w) => w.id)).size).toBe(3)
    expect(r.widgets[0]).toMatchObject({ id: 'x', x: 0, w: 12, h: 24 })
    expect(r.widgets[2].title).toBe('T')
    expect(r.widgets[2].settings).toBeUndefined()
    expect(r.widgets.some((a) => r.widgets.some((b) => overlaps(a, b)))).toBe(false)
  })
})

describe('widget registry', () => {
  it('has a definition with a size for every type', () => {
    // the server's registry (internal/api/dashboardwidgets.go) has the same types: TestWidgetRegistryInSync
    expect(WIDGET_TYPES.length).toBe(30)
    for (const t of WIDGET_TYPES) {
      const d = WIDGETS[t]
      expect(d.w).toBeGreaterThan(0)
      expect(d.w).toBeLessThanOrEqual(12)
      expect(d.icon).toMatch(/^pi /)
      for (const f of d.fields) if (f.kind === 'select') expect(f.options?.length).toBeGreaterThan(0)
    }
  })

  it('adds the project on the home dashboard', () => {
    expect(settingFields('coverage', true)[0].key).toBe('project_id')
    expect(settingFields('coverage', false)[0].key).toBe('chart')
    expect(settingFields('note', true).map((f) => f.key)).toEqual(['text'])
    expect(settingFields('nope', true)).toEqual([])
  })

  it('reads settings with defaults', () => {
    expect(setting({ type: 'breakdown', settings: {} }, 'dimension')).toBe('requirement_status')
    expect(setting({ type: 'breakdown', settings: { dimension: 'risk_level' } }, 'dimension')).toBe('risk_level')
    expect(setting({ type: 'breakdown' }, 'nothing')).toBe('')
    expect(cleanSettings({ a: '', b: null, c: 0, d: 'x' })).toEqual({ c: 0, d: 'x' })
    expect(cleanSettings({ a: '' })).toBeUndefined()
    expect(cleanSettings(undefined)).toBeUndefined()
  })

  it('offers the widgets the caller may see', () => {
    const all = availableWidgets(() => true)
    const none = availableWidgets(() => false)
    expect(all.length).toBe(WIDGET_TYPES.length)
    expect(none.some((d) => d.type === 'saved_query')).toBe(true)
    expect(none.some((d) => d.type === 'traceability_gaps')).toBe(false)
  })

  it('places new widgets in the first free spot', () => {
    const a = newWidget('coverage', [])
    expect(a).toMatchObject({ type: 'coverage', x: 0, y: 0, w: 6, h: 4 })
    const b = newWidget('coverage', [a], { chart: 'bar' })
    expect(b).toMatchObject({ x: 6, y: 0, settings: { chart: 'bar' } })
    expect(b.id).not.toBe(a.id)
    expect(newWidget('key_figures', [a, b])).toMatchObject({ x: 0, y: 4, w: 12 })
  })

  it('makes data requests keyed by what changes the data', () => {
    const w = { id: 'a', type: 'breakdown', x: 0, y: 0, w: 6, h: 4, title: 'T', settings: { chart: 'hbar', dimension: 'risk_level' } }
    const r = dataRequest(w, 'p1')
    expect(r.body).toEqual({ id: r.key, type: 'breakdown', project_id: 'p1', settings: { dimension: 'risk_level' } })
    expect(dataRequest({ ...w, settings: { dimension: 'risk_level', chart: 'bar' } }, 'p1').key).toBe(r.key)
    expect(dataRequest(w, 'p2').key).not.toBe(r.key)
    const home = dataRequest({ ...w, settings: { project_id: 'p3' } }, undefined)
    expect(home.body.project_id).toBe('p3')
    expect(dataRequest(w, 'p1', { dimension: 'test_status' }).body.settings).toEqual({ dimension: 'test_status' })
    expect(loadsData('note')).toBe(false)
    expect(loadsData('quality_metrics')).toBe(false)
    expect(loadsData('coverage')).toBe(true)
    expect(loadsData('nope')).toBe(false)
  })
})
