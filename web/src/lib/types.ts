// TypeScript mirrors of internal/model/model.go (JSON tags are authoritative).
import type { Applicability } from './variants'
import type { MfaStatus } from './mfa'
import type { FlakyTest, GroupCoverage } from './metrics'
import type { ScriptJob } from './webTests'

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
  /** has a confirmed second factor (user lists, /auth/me) */
  mfa_enabled?: boolean
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
  /** e-mail for notifications: off (default) | immediate | digest */
  email_notifications?: string
  // --- obligation keyword highlighting ---
  /** do not highlight obligation keywords in requirement texts (default: highlight) */
  hide_obligation_highlight?: boolean
  /** autocomplete in free-text fields: '' = on | history | project | off (lib/suggest.ts) */
  autocomplete?: string
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
  /** provisioned by SCIM: the identity provider manages role and membership */
  scim?: boolean
  /** has a confirmed second factor (members lists) */
  mfa_enabled?: boolean
  /** the MFA policy requires a second factor for this member's role here */
  mfa_required?: boolean
  /** open e-mail invitation (members list of the tenant) */
  invitation?: InvitationInfo
}

/** An open invitation of a member (Membership.invitation). */
export interface InvitationInfo {
  id: string
  sent_at: string
  expires_at: string
  sso: boolean
  expired: boolean
  invited_by: string
}

/** Scheduled e-mailing of a saved report. */
export interface ReportSubscription {
  id: string
  project_id: string
  definition_id: string
  definition_name?: string
  owner_id: string
  owner_email?: string
  frequency: string
  weekday: number
  day_of_month: number
  time: string
  timezone: string
  format: string
  recipients: string[]
  addresses: string[]
  message: string
  /** language the report is sent in (en, sv); empty = the owner's preference */
  lang?: string
  enabled: boolean
  next_run_at: string | null
  last_run_at: string | null
  last_sent_at: string | null
  last_error: string
  created_at: string
  updated_at: string
}

/** A member who can get reports by e-mail. */
export interface ReportRecipient {
  user_id: string
  email: string
  display_name: string
  role: string
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
  /** multi-factor state of the signed-in user (sessions only) */
  mfa?: MfaStatus
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
  /** generated document: an item it contains changed since (keys in outdated_items) */
  outdated?: boolean
  outdated_items?: string[] | null
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
  /** organisation (every member) or restricted (project members and organisation admins) */
  access?: ProjectAccessMode
  /** the caller's access to the project (effective permissions there) */
  my_access?: ProjectAccess
  created_at: string
  updated_at: string
}

export type ProjectAccessMode = 'organisation' | 'restricted'

/** The caller's access to a project (internal/model ProjectAccessInfo). */
export interface ProjectAccess {
  /** admin (organisation administrator), member, group or organisation (the organisation role) */
  source: 'admin' | 'member' | 'group' | 'organisation'
  /** the project role (member) or the groups' roles (group) */
  role?: string
  /** effective permissions in the project */
  permissions: string[]
  can_share: boolean
}

/** A user or SCIM group with a role in a project. */
export interface ProjectMember {
  project_id: string
  type: 'user' | 'group'
  id: string
  role: string
  email?: string
  display_name?: string
  avatar?: string
  org_role?: string
  added_by?: string
  created_at: string
  updated_at: string
}

/** GET /projects/{pid}/access */
export interface ProjectSharing {
  access: ProjectAccessMode
  members: number
  my_access: ProjectAccess | null
  grantable_roles: string[]
}

/** A user or group that can be added to a project. */
export interface MemberCandidate {
  type: 'user' | 'group'
  id: string
  email?: string
  display_name?: string
  avatar?: string
  org_role?: string
}

export type ItemKind = 'requirement' | 'test'

export interface Folder {
  id: string
  project_id: string
  parent_id: string | null
  kind: ItemKind
  name: string
  description: string
  /** '' or #rrggbb; subfolders without their own colour inherit it */
  color?: string
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
  /** the variants it applies to (absent: all variants) */
  applicability?: Applicability | null
  id: string
  /** custom field values by key (see lib/customFields) */
  custom_fields?: Record<string, string | number | boolean | string[] | null>
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
  /** catalogue id the item was copied from ("" otherwise) */
  catalog_ref?: string
  /** MITRE D3FEND defensive techniques it implements (D3-MFA) and MITRE EMB3D mitigations and threats it addresses (MID-026, TID-201) */
  d3fend?: string[]
  emb3d?: string[]
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
  /** unresolved discussion threads */
  open_discussions?: number
  relation_count?: number
  /** Related items in both directions (requirement relations and verifying tests). */
  relations?: ItemRef[] | null
  criteria_count?: number
  /** links to tests that are suspect (changed since the link was confirmed) */
  suspect_links?: number
  /** highest level of the open risks it addresses */
  risk_level?: string
  /** open defects linked to it or its tests */
  open_defects?: number
  /** open defects that block it / warn about it (the project's severity rules) */
  blocking_defects?: number
  defect_warnings?: number
  /** linked tests whose latest result is stale */
  stale_results?: number
  /** its relations that are suspect (the requirement at the other end changed) */
  suspect_relations?: number
  /** rolled-up verification state (see lib/verification.ts) */
  verification?: VerificationState
  /** criteria with passing evidence */
  criteria_verified?: number
  /** system elements it is allocated to */
  element_ids?: string[]
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
  /** the variants it applies to (absent: all variants) */
  applicability?: Applicability | null
  id: string
  /** custom field values by key (see lib/customFields) */
  custom_fields?: Record<string, string | number | boolean | string[] | null>
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
  /** the member responsible (e-mail address), or a name; "" none */
  owner?: string
  tags: string[] | null
  /** catalogue id the item was copied from ("" otherwise) */
  catalog_ref?: string
  /** MITRE D3FEND techniques whose implementation it verifies and MITRE EMB3D mitigations and threats it checks */
  d3fend?: string[]
  emb3d?: string[]
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
  /** unresolved discussion threads */
  open_discussions?: number
  called_by?: TestRef[] | null
  /** links to requirements that are suspect */
  suspect_links?: number
  /** pass/fail flips among the last results; flaky when at the threshold */
  flakiness?: number
  flaky?: boolean
  /** the latest result is stale (age, test_changed, requirement_changed) */
  stale?: boolean
  stale_reason?: string
  /** system elements it is allocated to */
  element_ids?: string[]
  /** Requirements it verifies, tests it calls and tests that call it. */
  relations?: ItemRef[] | null
  step_count?: number
  defects?: string[] | null
  /** Data-driven tests: parameter names and data rows (one iteration per row). */
  parameters?: string[] | null
  data_rows?: DataRow[] | null
}

/** One row of the test data of a data-driven test. */
export interface DataRow {
  name: string
  values: Record<string, string>
}

/** One coordinate of a configuration, e.g. Browser = Chrome. */
export interface DimensionValue {
  dimension: string
  value: string
}

/** A named combination of dimension values tests are executed against. */
export interface Configuration {
  id: string
  project_id: string
  name: string
  description: string
  values: DimensionValue[]
  position: number
  created_by?: string
  created_at?: string
  updated_at?: string
  /** runs that target it */
  run_count?: number
}

export interface ConfigurationRef {
  id: string
  name: string
}

/** The execution of one data row of a data-driven test. */
export interface Iteration {
  id: string
  position: number
  name: string
  values: Record<string, string>
  status: string
  notes: string
}

export interface TraceLink {
  id: string
  project_id: string
  requirement_id: string
  test_case_id: string
  criterion_id: string | null
  created_at: string
  /** set when the requirement or test changed after the link was confirmed */
  suspect_since?: string | null
  suspect_reason?: string
  requirement_key?: string
  test_case_key?: string
  /** the criteria the test verifies, each optionally through one step */
  criteria?: LinkCriterion[]
}

export interface TestRun {
  id: string
  /** custom field values by key (see lib/customFields) */
  custom_fields?: Record<string, string | number | boolean | string[] | null>
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
  release_id?: string | null
  release_name?: string
  summary?: Record<string, number>
  /** configurations the run targets (each test is executed once per configuration) */
  configurations?: ConfigurationRef[] | null
  /** the variant the run tests (null: variant-neutral) and the CI build of its results */
  variant_id?: string | null
  variant_name?: string
  build_id?: string | null
  build_label?: string
  /** the test cycle that created the run */
  cycle_id?: string | null
  cycle_name?: string
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
  /** the iteration (data row) of a data-driven execution */
  iteration_id?: string | null
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
  /** a link to an issue created from a requirement in the project's tracker */
  issue?: LinkedIssue
}

/** The tracker issue of an item link (internal/model LinkedIssue). */
export interface LinkedIssue {
  /** gitlab | github | jira */
  tracker: string
  /** group/project#12, owner/repo#12 or SHOP-12 */
  key: string
  /** the tracker's own status */
  status: string
  state: 'open' | 'closed' | string
  synced_at: string | null
  passed_at?: string
  /** the requirement changed after the issue was created or updated from it */
  outdated_since?: string | null
  outdated_reason?: string
}

/** GET /requirements/{id}/issue */
export interface RequirementIssueInfo {
  tracker: string
  link: ItemLink | null
  preview: { title: string; description: string; labels: string[] } | null
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
  /** the configuration it runs in (null without configuration) */
  configuration_id?: string | null
  configuration_name?: string
  /** iterations of a data-driven test (single executions) and their number (lists) */
  iterations?: Iteration[]
  iteration_count?: number
  /** the latest background job (automated tests run as jobs; GET /executions/{id}) */
  job?: ScriptJob | null
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
  /** suspect: the requirement at the other end changed (suspect_end: source | target) */
  suspect_since?: string | null
  suspect_reason?: string
  suspect_end?: string
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

/** Items that have discussions. */
export type DiscussionKind = 'requirement' | 'test' | 'defect' | 'risk' | 'run' | 'review'

export interface Comment {
  id: string
  project_id: string
  entity_type: DiscussionKind | string
  entity_id: string
  /** the first comment of the thread of a reply ('' starts a thread) */
  parent_id: string
  user_id: string
  user_email: string
  user_name: string
  /** avatar of the author (when the server includes it) */
  user_avatar?: string
  /** Markdown; empty when removed */
  body: string
  created_at: string
  updated_at: string
  resolved_at?: string | null
  resolved_by?: string
  resolved_by_name?: string
  deleted?: boolean
  deleted_at?: string
  /** user ids mentioned in the body */
  mentions?: string[]
  /** reaction → user ids */
  reactions?: Record<string, string[]>
}

/** A thread of a discussion list (GET /projects/{pid}/discussions). */
export interface DiscussionThread {
  root: Comment
  project_key: string
  entity_key: string
  entity_title: string
  replies: number
  last_at: string
  last_by: string
  participants: string[]
  mentioned: string[]
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
  /** Whether the server can send e-mail (email section of the configuration). */
  email_enabled?: boolean
  /** Hour of the day (UTC) when daily digests are sent. */
  email_digest_hour?: number
  /** Whether the server runs scripts (runner.enabled). */
  runner_enabled?: boolean
  /** Whether the server runs web tests with Playwright (runner.web.enabled). */
  web_tests_enabled?: boolean
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
  /** planned and in-progress runs, newest first */
  active_runs?: TestRun[] | null
  coverage_by_folder?: GroupCoverage[] | null
  coverage_by_category?: GroupCoverage[] | null
  defects_by_severity?: Record<string, number>
  flaky?: { total: number; items: FlakyTest[] | null; window: number; min_flips: number }
  /** change and defect correlations */
  correlations?: { blocked_requirements: number; warned_requirements: number; stale_results: number; suspect_relations: number }
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
  requirements: {
    id: string
    key: string
    title: string
    obligation: string
    coverage: string
    verification?: VerificationState
    criteria?: TraceCriterion[]
    records?: number
    parent_ids?: string[]
    blocked?: boolean
    stale_results?: number
  }[]
  tests: { id: string; key: string; title: string; last_result: string; stale?: boolean }[]
  links: TraceLink[]
  /** the configuration coverage and results are filtered to ('none': without configuration) */
  configuration?: string
}

/** Traceability rule kinds (internal/model TraceRuleKinds). */
export type TraceRuleKind = 'verified_by' | 'verifies' | 'relation' | 'item_link'

export interface TraceRuleFields {
  name: string
  description: string
  /** error | warning */
  severity: string
  enabled: boolean
  /** requirement | test */
  scope: string
  selector: string
  kind: TraceRuleKind | string
  min: number
  not_suspect?: boolean
  passed?: boolean
  relation_type?: string
  /** incoming | outgoing | '' (any) */
  direction?: string
  target_type?: string
  target_query?: string
  every?: boolean
  /** duplicate_suspect: least similarity (0.5–1) */
  threshold?: number
  /** unmitigated_risk: least risk level */
  level?: string
}

export interface TraceRule extends TraceRuleFields {
  id: string
  project_id: string
  position: number
  created_by: string
  created_at: string
  updated_at: string
}

export interface TraceViolation {
  kind: string
  id: string
  key: string
  title: string
  status?: string
  found: number
  required: number
  total: number
  /** built-in checks: reason code, detail, other items involved, fix actions */
  check?: string
  note?: string
  related?: TraceRelated[]
  fixes?: string[]
}

/** Another item involved in a violation of a built-in check. */
export interface TraceRelated {
  /** requirement | test | risk | release */
  kind: string
  id: string
  key: string
  title: string
  status?: string
  role: string
  /** id of the relation, trace link or prerequisite connecting it */
  link?: string
  check?: string
  note?: string
}

/** A built-in consistency check with its project default (GET trace-rules/builtins). */
export interface BuiltinCheck {
  kind: string
  scope: string
  name: string
  description: string
  severity: string
  enabled: boolean
  installed: boolean
  rule?: TraceRule
}

export interface TraceRuleResult {
  rule: TraceRule
  matched: number
  violations: TraceViolation[]
  violation_count: number
  passed: boolean
  error?: string
  waived?: number
}

export interface TraceRulesReport {
  results: TraceRuleResult[]
  passed: boolean
  summary: Record<string, number>
}

/** GET /projects/{pid}/analytics/consistency */
export interface ConsistencyReport extends TraceRulesReport {
  checks: BuiltinCheck[]
  installed: number
}

export interface ItemTraceViolation extends TraceViolation {
  rule: TraceRule
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
  /** true turns off "requirement status follows test results" */
  manual_verification?: boolean
}

// ---------------------------------------------------------------- releases

export interface ExitCriterion {
  /** coverage, passed, mandatory_passed, executed, no_failed, approved, no_suspect, no_open_defects, risks_mitigated, trace_rules, no_blocking_defects */
  type: string
  /** percent for percentage criteria */
  threshold?: number
  /** minimum severity (defects), risk level (risks) or rule severity (trace_rules: error | warning) */
  level?: string
  /** the test cycle of cycle_executed */
  cycle_id?: string
}

export interface Release {
  id: string
  project_id: string
  name: string
  version: string
  description: string
  /** planned, active, released, cancelled */
  status: string
  target_date: string
  exit_criteria: ExitCriterion[]
  released_at: string | null
  released_by: string
  release_note: string
  /** the variants the release targets (exit criteria evaluated per variant) */
  variant_ids?: string[]
  variants?: { id: string; name: string }[]
  created_by: string
  created_at: string
  updated_at: string
  requirement_ids?: string[]
  requirement_count: number
  run_count: number
}

export interface ReleaseItem {
  kind: string
  id: string
  key: string
  title: string
  note?: string
  /** combined criteria of a release gated per variant: the variants the item appears in */
  variants?: string[]
}

export interface CriterionResult extends ExitCriterion {
  met: boolean
  /** pass | waived | fail */
  state?: 'pass' | 'waived' | 'fail'

  actual: number
  total: number
  items: ReleaseItem[] | null
  available: boolean
  /** items accepted by a waiver (note: the waiver keys) */
  waived?: ReleaseItem[] | null
  /** risks above the threshold with a valid "accept risk" sign-off (residual_risk) */
  accepted?: ReleaseItem[] | null
}

export interface ReleaseStatusReport {
  release: Release
  go: boolean
  criteria: CriterionResult[]
  summary: Record<string, number>
  /** waivers in force for the release (lib/waivers Waiver) */
  waivers?: import('./waivers').Waiver[]
  risk_threshold?: string
  /** risks of the scope whose residual level is above the threshold (note: level, ", accepted") */
  residual_risks?: ReleaseItem[]
  /** the variant evaluated for (?variant=) */
  variant?: { id: string; name: string }
  /** a release that targets variants: the gate of each */
  variants?: VariantGate[]
}

export interface VariantGate {
  variant: { id: string; name: string }
  go: boolean
  criteria: CriterionResult[]
  summary: Record<string, number>
}

// ---------------------------------------------------------------- defects

export interface DefectLink {
  entity_type: string
  entity_id: string
  key?: string
  title?: string
}

export interface Defect {
  id: string
  /** custom field values by key (see lib/customFields) */
  custom_fields?: Record<string, string | number | boolean | string[] | null>
  project_id: string
  key: string
  title: string
  description: string
  /** open, in_progress, resolved, closed, rejected */
  status: string
  /** critical, major, minor, trivial */
  severity: string
  priority: string
  assignee: string
  external_system: string
  external_key: string
  external_url: string
  external_status: string
  synced_at: string | null
  created_by: string
  created_at: string
  updated_at: string
  resolved_at: string | null
  /** phase found in (requirements … production, '' unknown) */
  found_in?: string
  links?: DefectLink[]
}

export interface LinkedDefect extends Defect {
  /** how it is linked: requirement, test or execution */
  via: string
}

export interface DefectTracker {
  /** "", jira, github, gitlab or rt (Request Tracker) */
  type: string
  url: string
  project_key: string
  repo: string
  /** GitLab project ID or path */
  project?: string
  issue_type: string
  labels: string
  milestone?: string
  assignee?: string
  /** label templates of issues from requirements ("" default, "none") */
  requirement_labels?: string
  /** Jira issue type of issues from requirements */
  requirement_issue_type?: string
  /** RT queue (name or ID) */
  queue?: string
  /** RT custom field that receives the labels */
  labels_field?: string
  comment_on_pass?: boolean
  user: string
  /** write-only */
  token?: string
  has_token: boolean
}

/** The connections requirements are imported from (GET /projects/{pid}/import-sources). */
export interface ImportSources {
  jira: { url: string; user: string; jql: string; token?: string; has_token: boolean }
  azure_devops: { url: string; project: string; area_path: string; work_item_types: string; token?: string; has_token: boolean }
  /** the Jira import uses the defect tracker's connection */
  jira_from_tracker: boolean
  tracker_jira_url?: string
  tracker_jira_key?: string
}

// ---------------------------------------------------------------- risks

export interface Risk {
  id: string
  /** custom field values by key (see lib/customFields) */
  custom_fields?: Record<string, string | number | boolean | string[] | null>
  project_id: string
  key: string
  title: string
  description: string
  category: string
  likelihood: number
  impact: number
  /** identified, mitigating, mitigated, accepted, closed */
  status: string
  mitigation: string
  owner: string
  created_by: string
  created_at: string
  updated_at: string
  score: number
  /** low, medium, high, critical */
  level: string
  requirement_ids: string[]
  covered: number
  verified: number
  /** residual risk after mitigation (0 = not assessed; see lib/risks residualPoint) */
  residual_likelihood?: number
  residual_impact?: number
  residual_manual?: boolean
  residual_justification?: string
  review_date?: string
  review_owner?: string
  accepted_by?: string
  accepted_name?: string
  accepted_at?: string | null
  acceptance_comment?: string
  /** derived: the residual score and level in effect, and where they come from (inherent, verified, manual) */
  residual_score?: number
  residual_level?: string
  residual_source?: string
  threshold?: string
  above_threshold?: boolean
  /** the catalogue risk it was copied from ("" none) and that risk's catalogue area */
  catalog_ref?: string
  catalog_area?: string
  /** MITRE ATT&CK and ATLAS technique ids (T1110.004, T0886, AML.T0051) */
  attack?: string[]
  /** MITRE EMB3D threats that realise it (TID-201) */
  emb3d?: string[]
  acceptance_valid?: boolean
  review_due?: boolean
}

// ---------------------------------------------------------------- notifications

export interface AppNotification {
  id: string
  user_id: string
  project_id: string
  event: string
  title: string
  body: string
  entity_type: string
  entity_id: string
  actor: string
  read_at: string | null
  created_at: string
}

export interface Webhook {
  id: string
  project_id: string
  name: string
  url: string
  /** only returned when created or rotated */
  secret?: string
  has_secret: boolean
  events: string[]
  enabled: boolean
  created_by: string
  created_at: string
  updated_at: string
  last_delivery_at: string | null
  last_status: number
  last_error: string
}

// ---------------------------------------------------------------- reviews

export interface ChecklistItem {
  text: string
  checked: boolean
  note?: string
}

export interface ReviewRequest {
  id: string
  project_id: string
  entity_type: string
  entity_id: string
  entity_key: string
  entity_title: string
  entity_version: number
  reviewer_id: string
  reviewer_email: string
  reviewer_name: string
  requested_by: string
  message: string
  checklist: ChecklistItem[]
  /** pending, approved, changes_requested, cancelled */
  status: string
  comment: string
  due_date: string
  decided_at: string | null
  created_at: string
  updated_at: string
  outdated: boolean
}

export interface Signoff {
  id: string
  baseline_id: string
  user_id: string
  user_email: string
  user_name: string
  /** authored, reviewed, approved, released */
  meaning: string
  comment: string
  content_hash: string
  signed_at: string
  valid: boolean
}

// ---------- verification correlations (internal/model/verification.go) ----------

/** verified | partial | failed | pending | none */
export type VerificationState = 'verified' | 'partial' | 'failed' | 'pending' | 'none'

/** A criterion a trace link verifies (step_id: one step of the test). */
export interface LinkCriterion {
  criterion_id: string
  step_id?: string
}

/** A criterion row of the traceability matrix. */
export interface TraceCriterion {
  id: string
  position: number
  description: string
  method: string
  state: VerificationState
  records: number
}

/** One piece of evidence about a criterion or requirement. */
export interface Evidence {
  kind: 'test' | 'record'
  id: string
  link_id?: string
  /** test key, or the method of a record */
  key: string
  /** test title, or who performed a record */
  title: string
  step_id?: string
  step_position?: number
  result: string
  /** passed | failed | pending | void */
  state: 'passed' | 'failed' | 'pending' | 'void'
  plain?: boolean
  note?: string
  date?: string
}

export interface CriterionVerification {
  criterion_id: string
  position: number
  description: string
  method: string
  state: VerificationState
  evidence: Evidence[]
}

export interface VerificationReason {
  code: string
  params?: Record<string, string>
  text: string
}

/** GET /requirements/{id}/verification: is it verified, and why. */
export interface RequirementVerification {
  requirement_id: string
  key: string
  title: string
  status: string
  strict: boolean
  own: VerificationState
  state: VerificationState
  verified: boolean
  criteria: CriterionVerification[]
  evidence: Evidence[]
  criteria_verified: number
  criteria_total: number
  children: { id: string; key: string; title: string; status: string; state: VerificationState }[]
  children_verified: number
  parent_ids: string[]
  reasons: VerificationReason[]
  inconsistencies: VerificationReason[]
}

/** GET /projects/{pid}/verification */
export interface ProjectVerification {
  strict: boolean
  counts: Record<string, number>
  criteria_verified: number
  criteria_total: number
  requirements: RequirementVerification[]
  inconsistent: RequirementVerification[]
}

export interface VerificationRecord {
  id: string
  project_id: string
  requirement_id: string
  requirement_key?: string
  criterion_id: string | null
  criterion_text?: string
  method: string
  /** pass | fail | conditional */
  result: string
  performed_by: string
  performed_on: string
  note: string
  file_ids: string[]
  files: { id: string; name: string; kind: string }[]
  valid_until: string
  until_change: boolean
  invalidated_at: string | null
  invalidated_reason: string
  /** valid | expired | invalidated */
  validity: string
  signoff: { by: string; name: string; comment: string; at: string } | null
  created_by: string
  created_at: string
  updated_at: string
}

export interface VerificationSettings {
  strict_criteria: boolean
  /** debriefed exploratory sessions count as evidence for their scoped requirements */
  exploratory_evidence?: boolean
  updated_by?: string
  updated_at?: string | null
}

/** A node of the project's product breakdown. */
export interface SystemElement {
  id: string
  project_id: string
  parent_id: string | null
  name: string
  kind: string
  description: string
  position: number
  path?: string
  requirement_ids: string[]
  test_ids: string[]
  created_by: string
  created_at: string
  updated_at: string
}

/** Coverage and verification of the requirements allocated to an element or below it. */
export interface ElementCoverage {
  element_id: string
  parent_id: string | null
  name: string
  kind: string
  path: string
  depth: number
  requirements: number
  covered: number
  verified: number
  failed: number
  tests: number
  tests_passed: number
  percent: number
}
/** Quality metrics (GET /projects/{pid}/analytics/quality, internal/analytics quality.go). */
export interface QualityPeriod {
  period: string
  value: number
  count: number
  total: number
}
export interface QualityItem {
  kind: string
  id: string
  key: string
  title: string
  count: number
  value?: number
  note?: string
}
export interface QualityCount {
  value: string
  count: number
  percent: number
}
export interface QualityGroup {
  name: string
  requirements: number
  defects: number
  density: number
}
export interface QualityDurations {
  count: number
  mean_hours: number
  median_hours: number
  p90_hours: number
}
export interface QualityReport {
  from: string
  to: string
  period: 'week' | 'month'
  periods: string[]
  stale_days: number
  volatility: { changes: number; requirements: number; changed: number; per_requirement: number; trend: QualityPeriod[]; most_changed: QualityItem[] }
  defect_density: { defects: number; requirements: number; per_requirement: number; by_severity: QualityCount[]; by_requirement: QualityItem[]; by_folder: QualityGroup[]; by_category: QualityGroup[] }
  escaped: {
    defects: number
    escaped: number
    percent: number
    production: number
    after_release: number
    by_phase: QualityCount[]
    by_release: { id: string; name: string; released_at: string; escaped: number }[]
    trend: QualityPeriod[]
    items: QualityItem[]
  }
  effectiveness: { defects: number; tests: number; executions: number; hours: number; per_test: number; per_hour: number; top_tests: QualityItem[]; trend: QualityPeriod[]; untimed_percent: number }
  result_age: { tests: number; with_result: number; never: number; buckets: QualityCount[]; stale_days: number; stale: number; stale_percent: number; median_days: number; oldest: QualityItem[] }
  review_turnaround: QualityDurations & { pending: number; oldest_pending_days: number; trend: QualityPeriod[] }
  fix_time: QualityDurations & { open: number; by_severity: (QualityDurations & { severity: string })[]; trend: QualityPeriod[] }
  requirement_quality: { requirements: number; with_warnings: number; percent: number; by_rule: QualityCount[]; trend: QualityPeriod[]; items: QualityItem[] }
}

// ---------- test cycles (internal/model/cycles.go) ----------

export interface CycleProgress {
  planned: number
  executed: number
  passed: number
  failed: number
  blocked: number
  skipped: number
  in_progress: number
  not_run: number
  not_in_run: number
  unassigned: number
  estimated_minutes: number
  remaining_minutes: number
  actual_ms: number
  estimated_minutes_executed: number
  percent_executed: number
  percent_passed: number
}

export interface TestCycle {
  id: string
  project_id: string
  name: string
  goal: string
  description: string
  /** planned | active | closed */
  status: string
  owner_id: string
  release_id: string | null
  variant_id: string | null
  build_id: string | null
  environment: string
  configuration_ids: string[]
  start_date: string
  end_date: string
  scheduled_start_at: string | null
  schedule_id: string | null
  version: number
  started_at: string | null
  closed_at: string | null
  created_by: string
  created_at: string
  updated_at: string
  release_name?: string
  variant_name?: string
  build_label?: string
  run_ids: string[]
  progress: CycleProgress
}

export interface TestCycleTest {
  id: string
  cycle_id: string
  test_case_id: string
  test_key: string
  test_title: string
  configuration_id: string | null
  configuration_name?: string
  assignee_id: string
  estimate_minutes: number
  position: number
  added_by: string
  added_at: string
  execution_id: string | null
  run_id: string | null
  status: string
  executed_by?: string
  finished_at: string | null
  duration_ms: number
}

export interface CycleTesterLoad {
  assignee_id: string
  planned: number
  executed: number
  passed: number
  failed: number
  blocked: number
  estimated_minutes: number
  remaining_minutes: number
  actual_ms: number
}

export interface CycleBurndownPoint {
  day: string
  /** null after today */
  remaining: number | null
  remaining_minutes: number | null
  ideal: number
  ideal_minutes: number
}

export interface CycleReport {
  cycle: TestCycle
  testers: CycleTesterLoad[]
  burndown: CycleBurndownPoint[]
}

export interface CycleSchedule {
  id: string
  project_id: string
  name_pattern: string
  template_cycle_id: string | null
  template_name?: string
  query: string
  owner_id: string
  frequency: 'daily' | 'weekly' | 'monthly' | string
  weekday: number
  day_of_month: number
  time: string
  timezone: string
  duration_days: number
  auto_start: boolean
  enabled: boolean
  runs: number
  next_run_at: string | null
  last_run_at: string | null
  last_cycle_id: string | null
  last_error: string
  created_by: string
  created_at: string
  updated_at: string
}
