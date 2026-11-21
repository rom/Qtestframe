// Compliance with reference sources (GET /projects/{pid}/analytics/compliance,
// see internal/analytics/compliance.go): types and small display helpers.

export const COMPLIANCE_STATES = ['passed', 'failing', 'blocked', 'untested'] as const
export type ComplianceState = (typeof COMPLIANCE_STATES)[number]

export type Fulfilment = Record<ComplianceState, number>

export interface ComplianceObligation {
  /** must | should | may | other */
  obligation: string
  catalogue: number
  adopted: number
  missing: number
  own: number
  fulfilled: number
  percent: number
}

export interface ComplianceClause {
  /** "Art. 32", "A.8", "Annex I Part I" …; "" = the source as a whole */
  clause: string
  catalogue: number
  adopted: number
  missing: number
  own: number
  fulfilment: Fulfilment
  percent: number
}

export interface ComplianceGap {
  catalog_id: string
  area: string
  title: string
  obligation: string
  priority: string
  clauses: string[]
  refs: string[]
  tests: number
  /** rejected or obsolete project copies, which do not count */
  inactive?: { id: string; key: string; title: string; note?: string }[]
}

export interface ComplianceItem {
  id: string
  key: string
  title: string
  obligation: string
  state: ComplianceState
  catalog_ref?: string
  own: boolean
  clauses: string[]
}

export interface ComplianceSource {
  id: string
  kind: string
  jurisdiction?: string
  short?: string
  name: string
  url?: string
  catalogue: number
  adopted: number
  missing: number
  own: number
  fulfilment: Fulfilment
  adoption_percent: number
  percent: number
  /** -1 when the source has no MUST requirements */
  mandatory_percent: number
  obligations: ComplianceObligation[]
  clauses: ComplianceClause[]
  gaps: ComplianceGap[]
  items: ComplianceItem[]
  gaps_total: number
  items_total: number
}

export interface ComplianceReport {
  summary: { sources: number; catalogue: number; adopted: number; missing: number; own: number; fulfilment: Fulfilment; percent: number }
  sources: ComplianceSource[]
  unknown?: string[]
}

/** Colour class of a compliance percentage. */
export function complianceClass(percent: number): 'good' | 'fair' | 'poor' {
  return percent >= 80 ? 'good' : percent >= 50 ? 'fair' : 'poor'
}

/** Message key of the obligation group "other" (no MUST/SHOULD/MAY keyword). */
export const obligationLabelKey = 'analytics.compliance.otherObligation'
