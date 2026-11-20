// TypeScript mirrors of internal/model/model.go (JSON tags are authoritative).

export interface Tenant {
  id: string
  slug: string
  name: string
  created_at: string
}

export interface User {
  id: string
  email: string
  display_name: string
  super_admin: boolean
  auth_provider: string
  external_id?: string
  disabled: boolean
  created_at: string
  preferences?: Preferences | null
  /** "" (initials), "preset:<name>" or "file:<id>" */
  avatar?: string
  /** set after an administrator reset the password */
  must_change_password?: boolean
}

/** Per-user settings (all optional; empty = default). */
export interface Preferences {
  theme?: string
  /** en | sv; empty = browser language */
  language?: string | null
  start_page?: string
  default_project_id?: string
  default_tenant_id?: string
  page_size?: number
  date_format?: string
  timezone?: string
  compact_tables?: boolean
  hide_analysis?: boolean
  create_linked_test?: boolean | null
  /** first-login intro tour finished or skipped */
  intro_done?: boolean
}

export interface SessionInfo {
  id: string
  created_at: string
  expires_at: string
  last_seen: string | null
  ip: string
  user_agent: string
  tenant_id: string
  current: boolean
}

export interface Membership {
  tenant_id: string
  tenant_slug?: string
  tenant_name?: string
  user_id: string
  email?: string
  display_name?: string
  role: string
  created_at: string
  /** avatar of the member (when the server includes it) */
  avatar?: string
}

/** The sign-in before the current one (Me.previous_login). */
export interface PreviousLogin {
  created_at: string
  ip: string
  user_agent: string
  method: string
}

export interface Me {
  user: User
  tenant: Tenant | null
  role: string
  permissions: string[]
  memberships: Membership[]
  csrf_token: string
  previous_login?: PreviousLogin | null
  failed_logins_since_previous?: number
  first_login?: boolean
}

/** GET /auth/logins */
export interface LoginEvent {
  id: string
  user_id?: string
  success: boolean
  method: string
  ip: string
  user_agent: string
  created_at: string
}

export type FileKind = 'attachment' | 'document' | 'report' | 'avatar' | 'icon'

/** An item a file is attached to. */
export interface FileLink {
  entity_type: 'requirement' | 'test' | 'run' | 'execution' | string
  entity_id: string
  key: string
  /** Run of an execution or step link. */
  run_id?: string
  /** Execution of a step link (step evidence). */
  execution_id?: string
}

/** Metadata of a stored file (FileInfo). */
export interface FileInfo {
  id: string
  project_id: string | null
  kind: FileKind | string
  name: string
  content_type: string
  size: number
  sha256: string
  description: string
  tags: string[] | null
  uploaded_by: string
  created_at: string
  url: string
  links: FileLink[] | null
}

/** A saved custom report. */
export interface ReportDefinition {
  id: string
  project_id: string
  name: string
  description: string
  config: unknown
  created_by: string
  created_at: string
  updated_at: string
}

export interface AuthProvider {
  name: string
  display_name: string
  login_url: string
}

export interface Providers {
  local: boolean
  oidc: AuthProvider[] | null
  saml: AuthProvider[] | null
}

export interface APIToken {
  id: string
  tenant_id: string
  user_id: string
  name: string
  role: string
  prefix: string
  created_at: string
  expires_at?: string | null
  last_used?: string | null
}

export interface Project {
  id: string
  tenant_id: string
  key: string
  name: string
  description: string
  defect_url_template?: string
  /** "" (initials), "preset:<name>" or "file:<id>" */
  icon?: string
  /** "" (theme colour) or "#rrggbb" */
  color?: string
  /** offered for a requirement's source (the defaults unless custom_sources) */
  requirement_sources?: string[]
  custom_sources?: boolean
  /** general | software | hardware | system | business (software enables prompts) */
  type?: string
  /** technology of a software project, included in prompts */
  tech_context?: string
  created_at: string
  updated_at: string
}

export type ItemKind = 'requirement' | 'test'

export interface Folder {
  id: string
  project_id: string
  parent_id: string | null
  kind: ItemKind
  name: string
  description: string
  position: number
  created_at: string
}

export interface Category {
  id: string
  project_id: string
  scope: ItemKind
  dimension: string
  name: string
  color: string
  description: string
}

export interface EvaluationCriterion {
  id: string
  requirement_id?: string
  description: string
  method: string
  threshold: string
  position: number
}

export interface Requirement {
  id: string
  project_id: string
  folder_id: string | null
  key: string
  title: string
  description: string
  obligation: string
  type: string
  priority: string
  status: string
  source: string
  rationale: string
  owner: string
  tags: string[] | null
  version: number
  position: number
  category_ids: string[] | null
  criteria?: EvaluationCriterion[]
  created_by: string
  created_at: string
  updated_at: string
  folder_path?: string
  linked_tests: number
  coverage?: string
  comment_count?: number
  relation_count?: number
  /** Related items in both directions (requirement relations and verifying tests). */
  relations?: ItemRef[] | null
  criteria_count?: number
}

/** A reference from a list item to a related item (see lib/itemRefs.ts). */
export interface ItemRef {
  /** a relation type, "verifies" (test → requirement) or "calls" (test → test) */
  type: string
  direction: 'outgoing' | 'incoming' | string
  /** kind of the item at the other end */
  kind?: 'requirement' | 'test' | string
  id: string
  key: string
  title?: string
}

export interface TestStep {
  id: string
  position: number
  action: string
  expected: string
  data: string
  /** Shared step: the referenced test's steps are inserted here in runs. */
  call_test_id?: string | null
  call_test_key?: string
  call_test_title?: string
}

export interface TestRef {
  id: string
  key: string
  title: string
}

export interface TestCase {
  id: string
  project_id: string
  folder_id: string | null
  key: string
  title: string
  description: string
  preconditions: string
  execution_type: string
  script_language: string
  script: string
  expected_result: string
  priority: string
  status: string
  estimated_minutes: number
  tags: string[] | null
  position: number
  category_ids: string[] | null
  steps: TestStep[] | null
  version?: number
  created_by: string
  created_at: string
  updated_at: string
  folder_path?: string
  linked_requirements: number
  last_result?: string
  comment_count?: number
  called_by?: TestRef[] | null
  /** Requirements it verifies, tests it calls and tests that call it. */
  relations?: ItemRef[] | null
  step_count?: number
  defects?: string[] | null
}

export interface TraceLink {
  id: string
  project_id: string
  requirement_id: string
  test_case_id: string
  criterion_id: string | null
  created_at: string
  requirement_key?: string
  test_case_key?: string
}

export interface TestRun {
  id: string
  project_id: string
  name: string
  description: string
  status: string
  environment: string
  build: string
  created_by: string
  created_at: string
  started_at: string | null
  completed_at: string | null
  summary?: Record<string, number>
}

export interface StepResult {
  id: string
  step_id: string
  position: number
  action: string
  expected: string
  status: string
  actual: string
  /** Proof of the result (text); files are attachments of the step result. */
  evidence?: string
  attachment_count?: number
  /** Assumed: step data is echoed for convenience (falls back to test.steps). */
  data?: string
}

/** A reason why a test should not be executed yet (see Readiness). */
export interface ReadinessIssue {
  kind: 'test_status' | 'requirement_status' | 'dependency' | string
  id: string
  key: string
  title: string
  status: string
  /** blocking issues prevent execution; others are warnings */
  blocking: boolean
  /** statuses (or the result) that would clear the issue */
  required?: string[]
  /** the execution setting that decides whether it blocks */
  setting?: string
  /** prerequisites: the status is the result in the same run */
  in_run?: boolean
}

/** Whether a test may be executed (organisation settings, statuses, prerequisites). */
export interface Readiness {
  ready: boolean
  allowed: boolean
  issues: ReadinessIssue[]
}

/** A link from a requirement, test or run to another item or a web address. */
export interface ItemLink {
  id: string
  project_id: string
  source_type: string
  source_id: string
  /** requirement | test | run | file | report | url */
  target_type: string
  target_id: string | null
  url: string
  label: string
  created_by: string
  created_at: string
  source_key: string
  source_title: string
  target_key: string
  target_title: string
  /** files: document | report */
  target_kind?: string
  direction?: 'outgoing' | 'incoming' | string
}

/** A prerequisite between two tests (GET /tests/{id}/dependencies). */
export interface TestDependency {
  id: string
  project_id: string
  test_id: string
  depends_on_id: string
  created_by: string
  created_at: string
  test_key: string
  test_title: string
  depends_on_key: string
  depends_on_title: string
  direction?: 'outgoing' | 'incoming' | string
  other_last_result?: string
}

export interface Execution {
  id: string
  run_id: string
  test_case_id: string
  test_key: string
  test_title: string
  status: string
  assignee: string
  executed_by: string
  started_at: string | null
  finished_at: string | null
  duration_ms: number
  output: string
  notes: string
  position: number
  defects?: string[] | null
  step_results?: StepResult[]
  test?: TestCase
  /** Whether the test may be executed in this run (lists and single executions). */
  readiness?: Readiness
  run_name?: string
  run_status?: string
  run_environment?: string
  run_build?: string
  run_created_at?: string | null
}

export interface Relation {
  id: string
  project_id: string
  source_id: string
  target_id: string
  type: string
  created_by: string
  created_at: string
  source_key: string
  source_title: string
  target_key: string
  target_title: string
  direction?: 'outgoing' | 'incoming' | string
}

export interface Revision {
  id: string
  entity_type: 'requirement' | 'test' | string
  entity_id: string
  version: number
  action: string
  changed_fields: string[] | null
  snapshot: Record<string, unknown> | string | null
  user_email: string
  user_name?: string
  user_avatar?: string
  created_at: string
}

export interface BaselineItem {
  entity_type: 'requirement' | 'test' | string
  entity_id: string
  key: string
  title: string
  version: number
  snapshot?: unknown
}

export interface Baseline {
  id: string
  project_id: string
  name: string
  description: string
  created_by: string
  created_at: string
  requirement_count: number
  test_count: number
  items?: BaselineItem[] | null
}

export interface BaselineChange {
  entity_type: 'requirement' | 'test' | string
  entity_id: string
  key: string
  title: string
  from_version?: number
  to_version?: number
  changed_fields?: string[] | null
}

export interface BaselineDiff {
  from: string
  to: string
  added: BaselineChange[] | null
  removed: BaselineChange[] | null
  changed: BaselineChange[] | null
  unchanged: number
}

export interface Comment {
  id: string
  entity_type: string
  entity_id: string
  user_id: string
  user_email: string
  user_name: string
  /** avatar of the author (when the server includes it) */
  user_avatar?: string
  body: string
  created_at: string
  updated_at: string
}

export interface AuditEntry {
  id: string
  tenant_id: string
  user_id: string
  user_email: string
  user_name?: string
  user_avatar?: string
  action: string
  entity_type: string
  entity_id: string
  details: string
  created_at: string
}

export interface ListResponse<T> {
  items: T[]
  total: number
}

export interface Meta {
  obligations: string[]
  requirement_statuses: string[]
  requirement_types: string[]
  priorities: string[]
  test_statuses: string[]
  script_languages: string[]
  evaluation_methods: string[]
  run_statuses: string[]
  result_statuses: string[]
  roles: string[]
  /** Language packs of the requirement-language analysis. */
  analysis_languages: string[]
  /** Server default for creating a linked test with each new requirement. */
  auto_create_test?: boolean
}

export interface Dashboard {
  requirements: {
    total: number
    by_status: Record<string, number>
    by_obligation: Record<string, number>
    covered: number
    uncovered: number
  }
  tests: {
    total: number
    by_execution_type: Record<string, number>
    by_status: Record<string, number>
  }
  coverage: Record<string, number>
  runs: { total: number; by_status: Record<string, number> }
  recent_runs: TestRun[] | null
  results: Record<string, number>
}

export interface KeywordHit {
  keyword: string
  level: string
  offset: number
  length: number
}

export interface LanguageIssue {
  rule: string
  severity: string
  message: string
  offset: number
  length: number
  /** Values for the translated message (term, keyword, keywords, words, max, examples). */
  params?: Record<string, string>
}

export interface Analysis {
  obligation: string
  keywords: KeywordHit[] | null
  issues: LanguageIssue[] | null
  score: number
  /** Language pack used, e.g. "en" or "sv". */
  language?: string
}

export interface Traceability {
  requirements: { id: string; key: string; title: string; obligation: string; coverage: string }[]
  tests: { id: string; key: string; title: string; last_result: string }[]
  links: TraceLink[]
}

export interface SearchHit {
  type: 'requirement' | 'test' | 'run' | string
  id: string
  project_id: string
  project_key: string
  key: string
  title: string
  snippet: string
  matched_fields: string[] | null
}

export interface RbacInfo {
  roles: string[]
  matrix: Record<string, string[]>
  permissions?: string[]
}

/** A role of the organisation (GET /roles). */
export interface RoleInfo {
  name: string
  label?: string
  description?: string
  permissions: string[]
  builtin: boolean
  customized?: boolean
  editable: boolean
  deletable: boolean
  members: number
  tokens: number
}

/** Organisation settings (GET /settings). */
export interface TenantSettings {
  allow_unready_tests: boolean
  allow_unready_requirements: boolean
  ready_requirement_statuses: string[]
  allow_unmet_dependencies: boolean
}
