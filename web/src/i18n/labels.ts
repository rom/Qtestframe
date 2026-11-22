// Display labels for API enum values (statuses, priorities, roles …). The
// values sent to and received from the API stay unchanged.
import { reactive } from 'vue'
import { humanize, options } from '@/lib/format'
import { relationLabel } from '@/lib/relations'
import { t, te } from './index'

/** Labels of the organisation's custom roles (filled by stores/roles.ts). */
export const customRoleLabels = reactive(new Map<string, string>())

export type EnumGroup =
  | 'status'
  | 'result'
  | 'coverage'
  | 'verification'
  | 'obligation'
  | 'priority'
  | 'requirementType'
  | 'executionType'
  | 'scriptLanguage'
  | 'evaluationMethod'
  | 'role'
  | 'historyAction'
  | 'searchType'
  | 'auditAction'
  | 'entity'

/** Translated label of an enum value; unknown values are humanized. */
export function enumLabel(group: EnumGroup, value: string | null | undefined): string {
  if (!value) return ''
  const k = `enums.${group}.${value}`
  if (te(k)) return t(k)
  if (group === 'role' && customRoleLabels.has(value)) return customRoleLabels.get(value)!
  return humanize(value)
}

/** Translated labels of several roles, joined by ", " ("Tester, Subject matter expert"). */
export function rolesLabel(roles: readonly string[] | null | undefined): string {
  return (roles ?? []).map((r) => enumLabel('role', r)).join(', ')
}

/** Select options for enum values with translated labels. */
export function enumOptions(group: EnumGroup, values: readonly string[] | null | undefined): { label: string; value: string }[] {
  return options(values, (v) => enumLabel(group, v))
}

/** RFC 2119 keyword as shown in the obligation tag (always English): "must_not" → "MUST NOT". */
export function obligationKeyword(value: string | null | undefined): string {
  return (value ?? '').replace(/_/g, ' ').toUpperCase()
}

/** Explanation of an obligation keyword in the active language (tag tooltip). */
export function obligationHint(value: string | null | undefined): string {
  const k = `enums.obligationHint.${value ?? ''}`
  return value && te(k) ? t(k) : ''
}

/** Relation label seen from the queried requirement. */
export function relationText(type: string, direction: string | null | undefined): string {
  const k = `enums.relation.${type}.${direction === 'incoming' ? 'incoming' : 'outgoing'}`
  return te(k) ? t(k) : relationLabel(type, direction)
}
