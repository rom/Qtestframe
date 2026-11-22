// Generated documents: a requirements specification or a test plan, built by
// the server and saved in the document archive. See docs/api.md
// "Generated documents".

export const DOC_KINDS = ['requirements_spec', 'test_plan'] as const
export type DocKind = (typeof DOC_KINDS)[number]

export const DOC_FORMATS = ['pdf', 'docx', 'md'] as const
export type DocFormat = (typeof DOC_FORMATS)[number]

export const DOC_GROUPINGS = ['folder', 'category', 'type'] as const
export type DocGrouping = (typeof DOC_GROUPINGS)[number]

/** Fields of each requirement / test (internal/report RequirementDocFields, TestDocFields). */
export const DOC_FIELDS: Record<DocKind, readonly string[]> = {
  requirements_spec: ['description', 'rationale', 'criteria', 'type', 'priority', 'obligation', 'status', 'source', 'owner', 'tags', 'frameworks', 'categories', 'relations', 'tests', 'results', 'version', 'custom_fields'],
  test_plan: ['description', 'preconditions', 'steps', 'expected', 'priority', 'execution_type', 'status', 'estimate', 'tags', 'frameworks', 'categories', 'dependencies', 'requirements', 'data', 'results', 'version', 'custom_fields'],
}

/** Every field key of both kinds (for the message catalogue check). */
export const ALL_DOC_FIELDS = [...new Set([...DOC_FIELDS.requirements_spec, ...DOC_FIELDS.test_plan])]

/** The fields shown unless the user chooses: all but the latest results. */
export function defaultDocFields(kind: DocKind): string[] {
  return DOC_FIELDS[kind].filter((f) => f !== 'results')
}

/** Archive tag of the generated documents of a kind. */
export const DOC_TAGS: Record<DocKind, string> = { requirements_spec: 'requirements-specification', test_plan: 'test-plan' }

export interface DocForm {
  kind: DocKind
  format: DocFormat
  title: string
  version: string
  author: string
  query: string
  baselineId: string
  groupBy: DocGrouping
  dimension: string
  includeInactive: boolean
  fields: string[]
  traceability: boolean
  glossary: boolean
  /** appendix with the discussions of the items */
  discussions: boolean
  /** appendix with the relations between the requirements as graphs and a table (requirements specification) */
  dependencyGraph: boolean
  lang: string
}

export function blankDocForm(kind: DocKind, lang: string): DocForm {
  return {
    kind,
    format: 'pdf',
    title: '',
    version: '',
    author: '',
    query: '',
    baselineId: '',
    groupBy: 'folder',
    dimension: '',
    includeInactive: false,
    fields: defaultDocFields(kind),
    traceability: true,
    glossary: true,
    discussions: false,
    dependencyGraph: true,
    lang,
  }
}

/** Switches the kind: the default fields of the new kind (keeping the latest-results choice) and no filter (it is for the other kind of item). */
export function switchDocKind(form: DocForm, kind: DocKind): DocForm {
  if (form.kind === kind) return form
  const fields = defaultDocFields(kind)
  if (form.fields.includes('results')) fields.push('results')
  return { ...form, kind, fields: DOC_FIELDS[kind].filter((f) => fields.includes(f)), query: '' }
}

/** Body of POST /projects/{pid}/documents/generate. */
export function generateDocBody(form: DocForm) {
  const options: Record<string, unknown> = {
    fields: DOC_FIELDS[form.kind].filter((f) => form.fields.includes(f)),
    include_inactive: form.includeInactive,
    traceability: form.traceability,
    glossary: form.glossary,
    lang: form.lang,
  }
  if (form.discussions) options.include_discussions = true
  const text = { title: form.title, version: form.version, author: form.author, query: form.query, baseline_id: form.baselineId }
  for (const [k, v] of Object.entries(text)) if (v.trim()) options[k] = v.trim()
  if (form.kind === 'requirements_spec') {
    options.group_by = form.groupBy
    if (form.groupBy === 'category' && form.dimension) options.dimension = form.dimension
    options.dependency_graph = form.dependencyGraph
  }
  return { kind: form.kind, format: form.format, options }
}

/** Problems that keep the form from being sent (message keys under documents.generate.problem). */
export function docFormProblems(form: DocForm): string[] {
  const out: string[] = []
  if (!form.fields.length) out.push('fields')
  if (form.title.length > 200) out.push('title')
  if (form.version.length > 40) out.push('version')
  return out
}
