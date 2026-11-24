// Importing tests from other test management tools (TestImportDialog):
// the sources and their file formats, the types of the preview and apply
// responses (internal/api/testimport.go) and pure helpers.

export type TestImportSource = 'testrail' | 'xray' | 'zephyr' | 'qtest' | 'azure'
export type TestImportAction = 'new' | 'update' | 'unchanged' | 'conflict' | 'error'

export interface TestImportSourceInfo {
  id: TestImportSource
  name: string
  /** file extensions the source exports (accept attribute) */
  accept: string
  /** formats, for the hint */
  formats: string[]
  /** whether the export can carry past results */
  history: boolean
}

export const TEST_IMPORT_SOURCES: readonly TestImportSourceInfo[] = [
  { id: 'testrail', name: 'TestRail', accept: '.csv,.xml', formats: ['CSV', 'XML'], history: true },
  { id: 'xray', name: 'Xray', accept: '.csv,.json', formats: ['CSV', 'JSON'], history: true },
  { id: 'zephyr', name: 'Zephyr Scale / Squad', accept: '.csv,.xml', formats: ['CSV', 'XML'], history: false },
  { id: 'qtest', name: 'qTest', accept: '.xlsx,.csv', formats: ['XLSX', 'CSV'], history: false },
  { id: 'azure', name: 'Azure Test Plans', accept: '.csv', formats: ['CSV'], history: false },
]

/** Row actions in the order the preview shows them. */
export const TEST_IMPORT_ACTIONS: readonly TestImportAction[] = ['new', 'update', 'conflict', 'error', 'unchanged']

/** Largest accepted file (server limit, MB). */
export const TEST_IMPORT_MAX_MB = 20

export interface TestImportRow {
  index: number
  location: string
  external_id: string
  title: string
  folder: string
  type: string
  priority: string
  status: string
  steps: number
  action: TestImportAction
  id?: string
  key?: string
  links: string[]
  unresolved: string[]
  executions: number
  warnings: string[]
  errors: string[]
}

export interface TestImportValue {
  value: string
  count: number
  target: string
  guessed: boolean
}

export interface TestImportField {
  name: string
  count: number
  sample: string
  target: string
  guessed: boolean
}

export interface TestImportPreview {
  source: TestImportSource
  source_name: string
  format: string
  file: string
  target_folder: string
  total: number
  summary: Record<TestImportAction, number>
  rows: TestImportRow[]
  priorities: TestImportValue[]
  statuses: TestImportValue[]
  fields: TestImportField[]
  custom_fields: { key: string; label: string; type: string }[]
  priority_options: string[]
  status_options: string[]
  history: { runs: number; executions: number; known: number }
  unresolved_links: string[]
  previous: number
  warnings: string[]
  errors: string[]
}

export interface TestImportResult {
  created: number
  updated: number
  unchanged: number
  skipped: number
  folders_created: number
  links_added: number
  runs_created: number
  results_imported: number
  unresolved_links: string[]
  items: { index: number; id: string; key: string; title: string; action: 'created' | 'updated' }[]
  skipped_rows: { index: number; location: string; title: string; reason: string }[]
  warnings: string[]
}

export function sourceInfo(id: TestImportSource): TestImportSourceInfo {
  return TEST_IMPORT_SOURCES.find((s) => s.id === id) ?? TEST_IMPORT_SOURCES[0]
}

/** Whether a file name has an extension the source exports. */
export function acceptsTestFile(source: TestImportSource, name: string): boolean {
  const ext = '.' + (name.toLowerCase().split('.').pop() ?? '')
  return sourceInfo(source).accept.split(',').includes(ext)
}

export interface TestImportOptions {
  overwrite: boolean
  create: boolean
}

/** Whether a row is saved with these options (before exclusions). */
export function savableRow(r: Pick<TestImportRow, 'action'>, o: TestImportOptions): boolean {
  switch (r.action) {
    case 'new':
      return o.create
    case 'update':
      return true
    case 'conflict':
      return o.overwrite
    default:
      return false
  }
}

/** Number of tests a save creates or updates. */
export function testSaveCount(rows: readonly TestImportRow[], o: TestImportOptions, excluded: ReadonlySet<number>): number {
  return rows.filter((r) => !excluded.has(r.index) && savableRow(r, o)).length
}

/** Rows shown for an action filter ([] = all but unchanged). */
export function visibleTestRows(rows: readonly TestImportRow[], actions: readonly TestImportAction[]): TestImportRow[] {
  const want = new Set<TestImportAction>(actions.length ? actions : ['new', 'update', 'conflict', 'error'])
  return rows.filter((r) => want.has(r.action))
}

export interface TestImportState {
  source: TestImportSource
  folderId: string | null
  priorities: Record<string, string>
  statuses: Record<string, string>
  fields: Record<string, string>
  createFolders: boolean
  links: boolean
  history: boolean
}

/** Form fields of preview (and, with apply, of apply): the mapping as
 * chosen (the targets of the last preview, with the user's changes). */
export function testImportFields(state: TestImportState, apply?: TestImportOptions & { excluded: ReadonlySet<number> }): Record<string, string | undefined> {
  const json = (v: Record<string, string>) => (Object.keys(v).length ? JSON.stringify(v) : undefined)
  const f: Record<string, string | undefined> = {
    source: state.source,
    folder_id: state.folderId ?? undefined,
    priorities: json(state.priorities),
    statuses: json(state.statuses),
    fields: json(state.fields),
  }
  if (apply) {
    f.create_folders = String(state.createFolders)
    f.link_requirements = String(state.links)
    f.history = String(state.history)
    f.overwrite_conflicts = String(apply.overwrite)
    f.create = String(apply.create)
    if (apply.excluded.size) f.exclude = JSON.stringify([...apply.excluded].sort((a, b) => a - b))
  }
  return f
}

/** The current target of each value (the choice, else the guess). */
export function targetsOf<T extends { target: string }>(values: readonly T[], key: (v: T) => string): Record<string, string> {
  return Object.fromEntries(values.map((v) => [key(v), v.target]))
}
