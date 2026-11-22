// Change impact in the UI: labels of the impact graph's edges and paths,
// and the gate every changing action goes through: a dry run of the change
// (?dry_run=impact), the confirmation dialog when something is affected
// (ImpactGateHost, mounted once in the project layout), then the change with
// the chosen ?impact=.
import { reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { relationText } from '@/i18n/labels'
import { pathText, RELATION_TYPES, type ChangeImpact, type ImpactChoice, type ImpactDryRun, type ImpactStep } from '@/lib/correlations'

/** Labels of the edges of impact paths. */
export function useImpactLabels() {
  const { t, te } = useI18n()
  function edgeLabel(edge: string, direction?: string): string {
    if ((RELATION_TYPES as readonly string[]).includes(edge)) return relationText(edge, direction)
    const k = direction === 'incoming' && te(`correlations.edge.${edge}_in`) ? `correlations.edge.${edge}_in` : `correlations.edge.${edge}`
    return te(k) ? t(k) : edge
  }
  return { edgeLabel, pathOf: (path?: ImpactStep[]) => pathText(path, edgeLabel) }
}

interface GateState {
  open: boolean
  impact: ChangeImpact | null
  projectId: string
  fields: string[]
  resolve: ((c: ImpactChoice | null) => void) | null
}

/** The state of the confirmation dialog of the gate (ImpactGateHost). */
export const impactGate = reactive<GateState>({ open: false, impact: null, projectId: '', fields: [], resolve: null })

/** Asks what to do with an impact; resolves the choice, or null when the user cancels. Nothing affected: suspect at once. */
export function confirmImpact(impact: ChangeImpact | null | undefined, projectId: string, fields: string[] = []): Promise<ImpactChoice | null> {
  if (!impact || !impact.total) return Promise.resolve('suspect')
  impactGate.resolve?.(null)
  return new Promise((resolve) => {
    Object.assign(impactGate, { open: true, impact, projectId, fields, resolve })
  })
}

/** Closes the dialog with a choice (or null: cancelled). */
export function settleImpact(choice: ImpactChoice | null) {
  const r = impactGate.resolve
  Object.assign(impactGate, { open: false, resolve: null })
  r?.(choice)
}

/**
 * Previews a change with its dry run and asks what to do with its impact.
 * Resolves the query parameters to send the change with ({ impact }), or
 * null when the user cancelled. A preview that fails does not stop the
 * change (the change itself reports its errors).
 */
export async function impactGateFor(
  projectId: string,
  dryRun: (params: { dry_run: 'impact' }) => Promise<ImpactDryRun | undefined>,
  fields: string[] = [],
): Promise<{ impact: ImpactChoice } | null> {
  let res: ImpactDryRun | undefined
  try {
    res = await dryRun({ dry_run: 'impact' })
  } catch {
    return { impact: 'suspect' }
  }
  const choice = await confirmImpact(res?.impact, projectId, fields)
  return choice ? { impact: choice } : null
}
