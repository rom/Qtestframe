// Context-sensitive help tips: the "i" next to a field or section shows a
// short explanation on hover or keyboard focus and links to the section of
// the user guide that describes it. Texts live in guide.<lang>.ts under
// `tips.<id>` (dots in ids become "_"); anchors are the heading slugs of the
// guide page in each language (checked by tips.test.ts).
import { helpPath } from './links'
import type { GuidePage } from './pages'

export interface TipTarget {
  page: GuidePage
  /** heading slug per language; a language without one links to the page top */
  anchor: Record<string, string>
}

const REQ_FIELDS = { page: 'requirements', anchor: { en: 'fields', sv: 'fält' } } as const
const TEST_FIELDS = { page: 'tests', anchor: { en: 'fields', sv: 'fält' } } as const
const CREATE_RUN = { page: 'runs', anchor: { en: 'create-a-run', sv: 'skapa-en-körning' } } as const
const RUNNER = { page: 'runs', anchor: { en: 'the-execution-runner', sv: 'exekveringsvyn' } } as const
const CUSTOM_REPORTS = { page: 'reports-and-export', anchor: { en: 'custom-reports', sv: 'egna-rapporter' } } as const
const CATEGORIES = { page: 'tests', anchor: { en: 'categories', sv: 'kategorier' } } as const
const DEFECTS = { page: 'runs', anchor: { en: 'defects', sv: 'defekter' } } as const

export const TIPS = {
  'req.obligation': { page: 'requirements', anchor: { en: 'obligation-levels', sv: 'kravnivåer' } },
  'req.status': REQ_FIELDS,
  'req.type': REQ_FIELDS,
  'req.priority': REQ_FIELDS,
  'req.owner': REQ_FIELDS,
  'req.source': REQ_FIELDS,
  'req.rationale': REQ_FIELDS,
  'req.folder': { page: 'requirements', anchor: { en: 'move-requirements-and-folders', sv: 'flytta-krav-och-mappar' } },
  'req.categories': CATEGORIES,
  'req.tags': REQ_FIELDS,
  'req.criteria': { page: 'requirements', anchor: { en: 'evaluation-criteria', sv: 'utvärderingskriterier' } },
  'req.analysis': { page: 'requirements', anchor: { en: 'the-language-analysis', sv: 'språkanalysen' } },
  'req.linkedTest': { page: 'requirements', anchor: { en: 'linked-tests', sv: 'länkade-testfall' } },
  'test.preconditions': TEST_FIELDS,
  'test.status': TEST_FIELDS,
  'test.estimate': TEST_FIELDS,
  'test.script': { page: 'tests', anchor: { en: 'automated-tests-scripts', sv: 'automatiserade-testfall-skript' } },
  'test.steps': { page: 'tests', anchor: { en: 'manual-tests-steps', sv: 'manuella-testfall-steg' } },
  'test.expected': { page: 'tests', anchor: { en: 'manual-tests-steps', sv: 'manuella-testfall-steg' } },
  'test.calls': { page: 'tests', anchor: { en: 'shared-steps', sv: 'delade-steg' } },
  'run.environment': CREATE_RUN,
  'run.build': CREATE_RUN,
  'run.include': CREATE_RUN,
  'runner.status': RUNNER,
  'runner.assignee': RUNNER,
  'runner.defects': DEFECTS,
  'runner.notes': RUNNER,
  'runner.evidence': { page: 'runs', anchor: { en: 'evidence', sv: 'bevis' } },
  'test.dependencies': { page: 'tests', anchor: { en: 'dependencies', sv: 'beroenden' } },
  'settings.readiness': { page: 'administration', anchor: { en: 'test-execution-settings', sv: 'inställningar-för-testexekvering' } },
  'baseline.filter': { page: 'baselines', anchor: { en: 'create-a-baseline', sv: 'skapa-en-baslinje' } },
  'trace.onlyLinked': { page: 'traceability-and-coverage', anchor: { en: 'the-traceability-matrix', sv: 'spårbarhetsmatrisen' } },
  'trace.coverage': { page: 'traceability-and-coverage', anchor: { en: 'how-coverage-is-calculated', sv: 'så-beräknas-täckningen' } },
  'token.role': { page: 'administration', anchor: { en: 'api-tokens', sv: 'api-nycklar' } },
  'token.expires': { page: 'administration', anchor: { en: 'api-tokens', sv: 'api-nycklar' } },
  'member.role': { page: 'administration', anchor: { en: 'roles-and-permissions', sv: 'roller-och-behörigheter' } },
  'member.initialPassword': { page: 'administration', anchor: { en: 'members', sv: 'medlemmar' } },
  'report.groupBy': CUSTOM_REPORTS,
  'report.columns': CUSTOM_REPORTS,
  'export.aspects': { page: 'reports-and-export', anchor: { en: 'export', sv: 'export' } },
  'query.filter': { page: 'search-and-filters', anchor: { en: 'syntax-in-five-minutes', sv: 'syntaxen-på-fem-minuter' } },
  'settings.defectTemplate': DEFECTS,
  'settings.dimension': CATEGORIES,
} satisfies Record<string, TipTarget>

export type TipId = keyof typeof TIPS

export const TIP_IDS = Object.keys(TIPS) as TipId[]

/** The i18n key of a tip's text ("req.obligation" → "tips.req_obligation"). */
export function tipKey(id: string): string {
  return 'tips.' + id.replace(/\./g, '_')
}

/** Router path of the guide section a tip refers to (in `lang`, falling back to the page top). */
export function tipHelpPath(id: string, lang: string): string | null {
  const target = (TIPS as Record<string, TipTarget>)[id]
  if (!target) return null
  return helpPath(target.page, target.anchor[lang] ?? '')
}

export type TipPlacement = { top: number; left: number; above: boolean }

/**
 * Where to put a tip bubble of `size` next to the trigger `rect` in a
 * viewport of `vw` × `vh`: below the trigger when it fits, else above (or
 * wherever there is more room), horizontally aligned with the trigger and
 * kept `gutter` px inside the viewport.
 */
export function placeTip(
  rect: { top: number; bottom: number; left: number; width: number },
  size: { width: number; height: number },
  vw: number,
  vh: number,
  gutter = 8,
  gap = 6,
): TipPlacement {
  const spaceBelow = vh - rect.bottom - gap - gutter
  const spaceAbove = rect.top - gap - gutter
  const above = size.height > spaceBelow && spaceAbove > spaceBelow
  const top = above ? Math.max(gutter, rect.top - gap - size.height) : Math.min(rect.bottom + gap, Math.max(gutter, vh - gutter - size.height))
  const centre = rect.left + rect.width / 2
  const maxLeft = Math.max(gutter, vw - gutter - size.width)
  const left = Math.min(Math.max(gutter, centre - size.width / 2), maxLeft)
  return { top: Math.round(top), left: Math.round(left), above }
}

/** Guide section on the query language, linked from the query help popover. */
export function queryGuidePath(lang: string): string {
  return tipHelpPath('query.filter', lang) ?? helpPath('search-and-filters')
}
