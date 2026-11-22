// Statement of Applicability (mirrors internal/soa): the statement's rows,
// the implementation states, filters and the spreadsheet columns of the
// SoA table (lib/sheet fields for components/SpreadsheetGrid).
import type { Severity } from './colors'
import { matchOption, type Parsed, type SheetField, type SheetOption } from './sheet'
import type { SheetRow } from './sheetFields'

export const SOA_STATUSES = ['implemented', 'partial', 'planned', 'not_implemented'] as const
/** Applicability values in the table: yes, no or not decided. */
export const APPLICABILITY = ['yes', 'no', ''] as const

export interface SoASource {
  id: string
  kind: string
  short?: string
  name: string
  title: string
  shipped: boolean
  controls: number
}

export interface SoARequirement {
  id: string
  key: string
  title: string
  state: string
  catalog_ref?: string
  waiver?: string
}

export interface SoAEvidence {
  id: string
  key: string
  title: string
  result: string
}

export interface SoARow {
  id: string
  title: string
  group: string
  group_title: string
  applicable: boolean | null
  justification: string
  owner: string
  updated_by?: string
  updated_at?: string
  status: string
  requirements: SoARequirement[]
  evidence: SoAEvidence[]
  catalogue: number
  missing: number
  suggested: boolean
}

export interface SoASummary {
  controls: number
  applicable: number
  excluded: number
  undecided: number
  implemented: number
  partial: number
  planned: number
  not_implemented: number
}

export interface SoAStatement {
  source: SoASource
  url?: string
  summary: SoASummary
  rows: SoARow[]
}

export interface SoADecision {
  control_id: string
  applicable: boolean | null
  justification: string
  owner: string
}

/** Tag severity of an implementation state. */
export function soaStatusSeverity(status: string): Severity {
  switch (status) {
    case 'implemented':
      return 'success'
    case 'partial':
      return 'info'
    case 'planned':
      return 'warn'
    case 'not_implemented':
      return 'danger'
  }
  return 'secondary'
}

export const applicabilityOf = (v: boolean | null | undefined): (typeof APPLICABILITY)[number] => (v === true ? 'yes' : v === false ? 'no' : '')
export const applicableValue = (v: string): boolean | null => (v === 'yes' ? true : v === 'no' ? false : null)

export interface SoAFilter {
  text?: string
  /** yes, no, "" (undecided) */
  applicability?: string[]
  statuses?: string[]
  group?: string
}

export function filterSoARows(rows: readonly SoARow[], f: SoAFilter): SoARow[] {
  const q = (f.text ?? '').trim().toLowerCase()
  return rows.filter((r) => {
    if (f.applicability?.length && !f.applicability.includes(applicabilityOf(r.applicable))) return false
    if (f.statuses?.length && !f.statuses.includes(r.status)) return false
    if (f.group && r.group !== f.group) return false
    if (!q) return true
    return [r.id, r.title, r.group_title, r.justification, r.owner, ...r.requirements.map((x) => x.key)].join(' ').toLowerCase().includes(q)
  })
}

/** The control groups in order of first appearance. */
export function soaGroups(rows: readonly SoARow[]): { id: string; title: string }[] {
  const out: { id: string; title: string }[] = []
  for (const r of rows) if (r.group && !out.some((g) => g.id === r.group)) out.push({ id: r.group, title: r.group_title })
  return out
}

/** A row as a spreadsheet row (key = control id). */
export function soaSheetRow(r: SoARow): SheetRow {
  return {
    id: r.id,
    key: r.id,
    title: r.title || r.group_title,
    applicable: applicabilityOf(r.applicable),
    justification: r.justification,
    owner: r.owner,
    status: r.status,
    requirements: r.requirements.map((x) => (x.waiver ? `${x.key} (${x.waiver})` : x.key)).join(', '),
    evidence: r.evidence.map((e) => e.key).join(', '),
  }
}

type T = (key: string, params?: Record<string, unknown>) => string

/** The spreadsheet columns of the SoA table; applicability, justification and owner can be edited. */
export function soaSheetFields(t: T, canWrite: boolean): SheetField<SheetRow>[] {
  const s = (v: unknown) => (v === null || v === undefined ? '' : String(v))
  const ok = (value: unknown): Parsed => ({ ok: true, value })
  const fail = (error: string): Parsed => ({ ok: false, error })
  const ro = (id: string, header: string, width: number, text: (r: SheetRow) => string = (r) => s(r[id])): SheetField<SheetRow> => ({ id, header, width, editor: 'none', text })
  const appOptions = (): SheetOption[] => APPLICABILITY.map((v) => ({ value: v, label: t(`soa.applicability.${v || 'undecided'}`) }))
  return [
    { ...ro('key', t('soa.col.control'), 110), fixed: true },
    { ...ro('title', t('soa.col.title'), 300), fixed: true },
    {
      id: 'applicable',
      header: t('soa.col.applicable'),
      width: 130,
      editor: 'select',
      clearable: true,
      text: (r) => t(`soa.applicability.${s(r.applicable) || 'undecided'}`),
      value: (r) => s(r.applicable),
      options: appOptions,
      editable: () => canWrite,
      parse: (raw) => {
        if (!raw.trim()) return ok('')
        const o = matchOption(appOptions(), raw)
        return o ? ok(o.value) : fail(t('soa.err.applicability', { value: raw.trim() }))
      },
    },
    {
      id: 'justification',
      header: t('soa.col.justification'),
      width: 320,
      editor: 'textarea',
      clearable: true,
      text: (r) => s(r.justification),
      value: (r) => s(r.justification),
      editable: () => canWrite,
      parse: (raw) => (raw.length > 4000 ? fail(t('soa.err.tooLong', { max: 4000 })) : ok(raw.replace(/\r\n?/g, '\n').trim())),
    },
    {
      id: 'owner',
      header: t('soa.col.owner'),
      width: 160,
      editor: 'text',
      clearable: true,
      text: (r) => s(r.owner),
      value: (r) => s(r.owner),
      editable: () => canWrite,
      parse: (raw) => (raw.length > 200 ? fail(t('soa.err.tooLong', { max: 200 })) : ok(raw.replace(/[\r\n]+/g, ' ').trim())),
    },
    ro('status', t('soa.col.status'), 170, (r) => (s(r.status) ? t(`soa.status.${s(r.status)}`) : '—')),
    ro('requirements', t('soa.col.requirements'), 220),
    ro('evidence', t('soa.col.evidence'), 200),
  ]
}

/**
 * The decisions to save for spreadsheet changes: the rows' current
 * decisions with the changed cells applied (one decision per control).
 */
export function decisionsFor(rows: readonly SoARow[], changes: readonly { row: number; field: string; value: unknown }[], shown: readonly SheetRow[]): SoADecision[] {
  const byId = new Map(rows.map((r) => [r.id, r]))
  const out = new Map<string, SoADecision>()
  for (const c of changes) {
    const id = shown[c.row]?.id
    const r = id ? byId.get(id) : undefined
    if (!r) continue
    const d = out.get(r.id) ?? { control_id: r.id, applicable: r.applicable, justification: r.justification, owner: r.owner }
    if (c.field === 'applicable') d.applicable = applicableValue(String(c.value ?? ''))
    if (c.field === 'justification') d.justification = String(c.value ?? '')
    if (c.field === 'owner') d.owner = String(c.value ?? '')
    out.set(r.id, d)
  }
  return [...out.values()]
}

/** Decisions that exclude a control without a justification (the server refuses them). */
export function unjustifiedExclusions(list: readonly SoADecision[]): string[] {
  return list.filter((d) => d.applicable === false && d.justification.trim().length < 3).map((d) => d.control_id)
}
