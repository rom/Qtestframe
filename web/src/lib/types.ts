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
}

export interface Me {
  user: User
  tenant: Tenant | null
  role: string
  permissions: string[]
  memberships: Membership[]
  csrf_token: string
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
}

export interface TestStep {
  id: string
  position: number
  action: string
  expected: string
  data: string
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
  created_by: string
  created_at: string
  updated_at: string
  folder_path?: string
  linked_requirements: number
  last_result?: string
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
  /** Assumed: step data is echoed for convenience (falls back to test.steps). */
  data?: string
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
  step_results?: StepResult[]
  test?: TestCase
}

export interface AuditEntry {
  id: string
  tenant_id: string
  user_id: string
  user_email: string
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
}

export interface Analysis {
  obligation: string
  keywords: KeywordHit[] | null
  issues: LanguageIssue[] | null
  score: number
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
}
