import { describe, expect, it } from 'vitest'
import { applicabilityOf, applicableValue, decisionsFor, filterSoARows, soaGroups, soaSheetFields, soaSheetRow, soaStatusSeverity, unjustifiedExclusions, SOA_STATUSES, type SoARow } from './soa'
import { isEditable, planCell } from './sheet'

const row = (over: Partial<SoARow> = {}): SoARow => ({
  id: 'A.8.5',
  title: 'Secure authentication',
  group: 'A.8',
  group_title: 'Technological controls',
  applicable: null,
  justification: '',
  owner: '',
  status: 'planned',
  requirements: [{ id: 'r1', key: 'P-REQ-1', title: 'MFA', state: 'untested' }],
  evidence: [{ id: 't1', key: 'P-TC-1', title: 'MFA works', result: '' }],
  catalogue: 3,
  missing: 1,
  suggested: true,
  ...over,
})
const t = (k: string, p?: Record<string, unknown>) => (p ? `${k} ${JSON.stringify(p)}` : k)

describe('soa helpers', () => {
  it('maps applicability both ways', () => {
    expect([true, false, null, undefined].map(applicabilityOf)).toEqual(['yes', 'no', '', ''])
    expect(['yes', 'no', '', 'x'].map(applicableValue)).toEqual([true, false, null, null])
    for (const s of SOA_STATUSES) expect(soaStatusSeverity(s)).not.toBe('secondary')
    expect(soaStatusSeverity('')).toBe('secondary')
  })
  it('filters rows and lists groups', () => {
    const rows = [row(), row({ id: 'A.5.1', title: 'Policies', group: 'A.5', group_title: 'Organizational controls', applicable: true, status: 'implemented', owner: 'CISO' }), row({ id: 'A.7.1', group: 'A.7', applicable: false, status: '' })]
    expect(filterSoARows(rows, { text: 'ciso' }).map((r) => r.id)).toEqual(['A.5.1'])
    expect(filterSoARows(rows, { text: 'p-req-1' }).length).toBe(3)
    expect(filterSoARows(rows, { applicability: [''] }).map((r) => r.id)).toEqual(['A.8.5'])
    expect(filterSoARows(rows, { applicability: ['no'] }).map((r) => r.id)).toEqual(['A.7.1'])
    expect(filterSoARows(rows, { statuses: ['implemented'] }).map((r) => r.id)).toEqual(['A.5.1'])
    expect(filterSoARows(rows, { group: 'A.8' }).map((r) => r.id)).toEqual(['A.8.5'])
    expect(soaGroups(rows).map((g) => g.id)).toEqual(['A.8', 'A.5', 'A.7'])
  })
  it('makes sheet rows and decisions from edits', () => {
    const rows = [row(), row({ id: 'A.7.1', applicable: true, justification: 'x', owner: 'o', requirements: [{ id: 'r2', key: 'P-REQ-2', title: '', state: 'passed', waiver: 'P-WVR-1' }] })]
    const sheet = rows.map(soaSheetRow)
    expect(sheet[0]).toMatchObject({ id: 'A.8.5', key: 'A.8.5', applicable: '', requirements: 'P-REQ-1', evidence: 'P-TC-1' })
    expect(sheet[1].requirements).toBe('P-REQ-2 (P-WVR-1)')
    const ds = decisionsFor(rows, [
      { row: 0, field: 'applicable', value: 'no' },
      { row: 0, field: 'justification', value: 'No such systems' },
      { row: 1, field: 'owner', value: 'CISO' },
      { row: 9, field: 'owner', value: 'x' },
    ], sheet)
    expect(ds).toEqual([
      { control_id: 'A.8.5', applicable: false, justification: 'No such systems', owner: '' },
      { control_id: 'A.7.1', applicable: true, justification: 'x', owner: 'CISO' },
    ])
    expect(unjustifiedExclusions([{ control_id: 'A', applicable: false, justification: ' ', owner: '' }, ...ds])).toEqual(['A'])
  })
  it('has spreadsheet columns that parse typed text', () => {
    const fields = soaSheetFields(t, true)
    expect(fields.map((f) => f.id)).toEqual(['key', 'title', 'applicable', 'justification', 'owner', 'status', 'requirements', 'evidence'])
    const sheet = [soaSheetRow(row())]
    const ctx = { rows: sheet, fields, readOnly: () => 'read only' }
    const app = fields.findIndex((f) => f.id === 'applicable')
    expect(planCell(ctx, 0, app, 'soa.applicability.no').change?.value).toBe('no')
    expect(planCell(ctx, 0, app, 'yes').change?.value).toBe('yes')
    expect(planCell(ctx, 0, app, 'maybe').error?.message).toContain('soa.err.applicability')
    expect(planCell(ctx, 0, fields.findIndex((f) => f.id === 'owner'), ' CISO\n').change?.value).toBe('CISO')
    expect(planCell(ctx, 0, fields.findIndex((f) => f.id === 'justification'), 'x'.repeat(4001)).error).toBeTruthy()
    expect(planCell(ctx, 0, fields.findIndex((f) => f.id === 'status'), 'x').error?.message).toBe('read only')
    expect(fields.find((f) => f.id === 'status')!.text(soaSheetRow(row({ status: '' })))).toBe('—')
    expect(soaSheetFields(t, false).filter((f) => isEditable(f, sheet[0])).length).toBe(0)
  })
})
