// Custom fields as list columns (requirements, tests, defects, risks, runs):
// column id cf.<key>, the label with its unit, sorting by the value.
import { cfField, cfKey, fieldLabel, sortKey, valueText, type CustomField } from './customFields'

export interface CustomColumn {
  field: string
  header: string
  width?: string
  sortable?: boolean
  /** a number for number fields (the table compares numbers numerically; typed as string for PrimeVue) */
  sortField: (row: { custom_fields?: Record<string, unknown> }) => string
}

export function customColumns(fields: readonly CustomField[]): CustomColumn[] {
  return fields.map((f) => ({
    field: cfField(f.key),
    header: fieldLabel(f),
    width: f.type === 'long_text' ? '260px' : undefined,
    sortField: (row) => sortKey(f, row.custom_fields?.[f.key]) as string,
  }))
}

/** Column ids of the fields shown in lists by default. */
export function listedFields(fields: readonly CustomField[]): string[] {
  return fields.filter((f) => f.show_in_list).map((f) => cfField(f.key))
}

/** The text of a custom field cell of a row. */
export function customCellText(fields: readonly CustomField[], row: { custom_fields?: Record<string, unknown> }, field: string, yesNo: [string, string]): string {
  const f = fields.find((x) => x.key === cfKey(field))
  return f ? valueText(f, row.custom_fields?.[f.key], yesNo) : ''
}
