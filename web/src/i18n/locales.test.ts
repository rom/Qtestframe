// Message catalogue checks: sv has exactly the keys of en, nothing is empty,
// placeholders and plural forms match, and every key the code uses exists.
import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import en from './locales/en'
import sv from './locales/sv'

type Tree = { [k: string]: string | Tree }

function flatten(obj: Tree, prefix = '', out: Record<string, string> = {}): Record<string, string> {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out[key] = v
    else flatten(v, key, out)
  }
  return out
}
const placeholders = (s: string) => [...new Set([...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]))].sort()
const forms = (s: string) => s.split(/\s\|\s/).length

const EN = flatten(en as unknown as Tree)
const SV = flatten(sv as unknown as Tree)

describe('en / sv parity', () => {
  it('has the same keys', () => {
    const missing = Object.keys(EN).filter((k) => !(k in SV))
    const extra = Object.keys(SV).filter((k) => !(k in EN))
    expect(missing).toEqual([])
    expect(extra).toEqual([])
  })
  it('has no empty messages', () => {
    const empty = [...Object.entries(EN), ...Object.entries(SV)].filter(([, v]) => !v.trim()).map(([k]) => k)
    expect(empty).toEqual([])
  })
  it('uses the same placeholders', () => {
    const diff = Object.keys(EN).filter((k) => placeholders(EN[k]).join() !== placeholders(SV[k] ?? '').join())
    expect(diff).toEqual([])
  })
  it('has the same number of plural forms', () => {
    const diff = Object.keys(EN).filter((k) => forms(EN[k]) !== forms(SV[k] ?? ''))
    expect(diff).toEqual([])
  })
  it('avoids characters with special meaning in vue-i18n', () => {
    // @ starts a linked message, $ and unbalanced braces break the parser
    const bad = [...Object.entries(EN), ...Object.entries(SV)].filter(([, v]) => /[@$]/.test(v) || (v.match(/\{/g)?.length ?? 0) !== (v.match(/\}/g)?.length ?? 0))
    expect(bad).toEqual([])
  })
  it('keeps RFC 2119 keywords in English in the Swedish obligation labels', () => {
    for (const k of ['must', 'shall', 'should', 'may']) expect(SV[`enums.obligation.${k}`]).toContain(k.toUpperCase())
  })
})

// ---------- keys used in the source ----------

// vitest runs from web/
const SRC = join(process.cwd(), 'src') + '/'
function files(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f)
    if (statSync(p).isDirectory()) return f === 'locales' ? [] : files(p)
    return /\.(vue|ts)$/.test(f) && !/\.test\.ts$/.test(f) ? [p] : []
  })
}
const sections = Object.keys(en).join('|')
// single-quoted literals ('nav.projects', also in ternaries) and i18n-t keypath="…"
const KEY_RE = new RegExp(`(?:'|keypath=")((?:${sections})\\.[\\w-]+(?:\\.[\\w-]+)*)['"]`, 'g')
// permission names look like keys ('run.read') but are not messages
// field keys of the autocomplete (lib/suggest) look like keys too ('run.name')
const SKIP = /(lib\/permissions|lib\/suggest|guide\/tips)\.ts$/ // ids, not message keys

describe('keys used in the code', () => {
  const used = new Map<string, string>()
  for (const f of files(SRC).filter((f) => !SKIP.test(f))) {
    for (const m of readFileSync(f, 'utf8').matchAll(KEY_RE)) used.set(m[1], f.slice(SRC.length))
  }
  it('finds a reasonable number of keys', () => {
    expect(used.size).toBeGreaterThan(300)
  })
  it('all exist in the catalogue', () => {
    const unknown = [...used].filter(([k]) => !(k in EN)).map(([k, f]) => `${k} (${f})`)
    expect(unknown).toEqual([])
  })
  it('has a page title for every named route', () => {
    const router = readFileSync(join(SRC, 'router.ts'), 'utf8')
    const names = [...router.matchAll(/name: '(\w+)', component/g)].map((m) => m[1]).filter((n) => n !== 'home')
    expect(names.length).toBeGreaterThan(15)
    expect(names.filter((n) => !(`titles.${n}` in EN))).toEqual([])
  })
})

describe('dynamic keys', () => {
  it('cover the query help', async () => {
    const { QUERY_HELP, QUERY_OPERATORS } = await import('@/lib/query')
    const missing = [...QUERY_HELP, ...QUERY_OPERATORS].filter((h) => !(`query.meaning.${h.id}` in EN)).map((h) => h.id)
    expect(missing).toEqual([])
  })
  it('cover every enum value the API knows', async () => {
    const { RELATION_TYPES } = await import('@/lib/relations')
    // values mirror internal/model (stores/meta.ts DEFAULT_META)
    const groups: Record<string, string[]> = {
      obligation: ['must', 'must_not', 'shall', 'shall_not', 'should', 'should_not', 'may', 'optional', 'will', 'describe'],
      obligationHint: ['must', 'must_not', 'shall', 'shall_not', 'should', 'should_not', 'may', 'optional', 'will', 'describe'],
      status: ['draft', 'review', 'approved', 'implemented', 'verified', 'rejected', 'obsolete', 'ready', 'deprecated', 'planned', 'in_progress', 'completed', 'aborted'],
      requirementType: ['functional', 'non_functional', 'security', 'performance', 'usability', 'interface', 'constraint', 'business', 'regulatory'],
      priority: ['critical', 'high', 'medium', 'low'],
      scriptLanguage: ['sh', 'bash', 'python', 'javascript', 'playwright'],
      evaluationMethod: ['test', 'inspection', 'analysis', 'demonstration', 'review'],
      result: ['not_run', 'in_progress', 'passed', 'failed', 'blocked', 'skipped'],
      coverage: ['none', 'not_run', 'passed', 'failed', 'partial'],
      role: ['admin', 'manager', 'tester', 'viewer'],
      executionType: ['manual', 'automated'],
    }
    const missing = Object.entries(groups).flatMap(([g, vs]) => vs.filter((v) => !(`enums.${g}.${v}` in EN)).map((v) => `${g}.${v}`))
    expect(missing).toEqual([])
    expect(RELATION_TYPES.filter((r) => !(`enums.relation.${r}.incoming` in EN))).toEqual([])
  })
  it('cover every colour theme and the theme picker', async () => {
    const { THEME_IDS } = await import('@/themes/registry')
    expect(THEME_IDS.filter((id) => !(`theme.${id}` in EN))).toEqual([])
    expect(['theme.picker', 'theme.unset'].filter((k) => !(k in EN))).toEqual([])
  })
  it('cover the dashboard insights', async () => {
    const { ACTIVITY_SERIES, AGE_BUCKETS, MITIGATION_ORDER, REVIEW_ORDER, SEVERITY_ORDER, LEVELS_DESC } = await import('@/lib/insights')
    const RELEASE_STATUSES = ['planned', 'active', 'released', 'cancelled'] // internal/model ReleaseStatuses
    const keys = [
      ...REVIEW_ORDER.map((k) => `dashboard.review.${k}`),
      ...MITIGATION_ORDER.map((k) => `dashboard.mitigation.${k}`),
      ...AGE_BUCKETS.map((k) => `dashboard.ageBucket.${k}`),
      ...ACTIVITY_SERIES.map((k) => `dashboard.activitySeries.${k}`),
      ...['rules', 'errors', 'warnings', 'violations'].map((k) => `dashboard.ruleCounts.${k}`),
      ...SEVERITY_ORDER.map((k) => `defectsView.severity.${k}`),
      ...LEVELS_DESC.map((k) => `risksView.level.${k}`),
      ...RELEASE_STATUSES.map((k) => `releasesView.status.${k}`),
    ]
    expect(keys.filter((k) => !(k in EN))).toEqual([])
  })
  it('cover the runner keyboard help', async () => {
    const { RUNNER_SHORTCUTS } = await import('@/lib/shortcuts')
    expect(RUNNER_SHORTCUTS.filter((s) => !(`runner.keys.${s.action}` in EN)).map((s) => s.action)).toEqual([])
  })
  it('cover the preset avatars, icons, permissions, report templates and file kinds', async () => {
    const { AVATAR_PRESETS, ICON_PRESETS } = await import('@/assets/presets')
    const { P, permissionKey } = await import('@/lib/permissions')
    const { KNOWN_TEMPLATES, REPORT_ENTITIES } = await import('@/lib/reports')
    const { ARCHIVE_KINDS } = await import('@/lib/files')
    const presets = [...Object.keys(AVATAR_PRESETS), ...Object.keys(ICON_PRESETS)].map((n) => `avatar.presets.${n.replace(/-/g, '_')}`)
    const perms = Object.values(P).map((p) => `rbac.perm.${permissionKey(p)}`)
    const tpls = KNOWN_TEMPLATES.flatMap((id) => [`reports.templates.${id}.name`, `reports.templates.${id}.description`])
    const ents = REPORT_ENTITIES.map((e) => `reports.entity.${e}`)
    const kinds = ARCHIVE_KINDS.map((k) => `documents.kind.${k}`)
    const problems = ['entity', 'columns', 'run', 'group_by', 'sort_by', 'title'].map((p) => `reports.problem.${p}`)
    expect([...presets, ...perms, ...tpls, ...ents, ...kinds, ...problems].filter((k) => !(k in EN))).toEqual([])
  })
  it('cover the key providers of stored secrets', async () => {
    const { SECRETS_INFO } = await import('@/lib/systemSettings')
    const keys = [
      ...['local', 'tpm', 'pkcs11'].map((p) => `sysSettings.secrets.provider.${p}`),
      ...SECRETS_INFO.map((i) => `sysSettings.secrets.info.${i}`),
      // values of info.key_source (internal/secrets)
      ...['file', 'environment', 'ephemeral'].map((s) => `sysSettings.secrets.source.${s}`),
    ]
    expect(keys.filter((k) => !(k in EN))).toEqual([])
  })
  it('cover the document generators', async () => {
    const { ALL_DOC_FIELDS, DOC_GROUPINGS, DOC_KINDS } = await import('@/lib/documents')
    const { LANGUAGES } = await import('./select')
    const { docFormProblems, blankDocForm } = await import('@/lib/documents')
    const problems = docFormProblems({ ...blankDocForm('test_plan', 'en'), fields: [], title: 'x'.repeat(300), version: 'x'.repeat(50) })
    const keys = [
      ...ALL_DOC_FIELDS.map((f) => `documents.generate.field.${f}`),
      ...DOC_KINDS.flatMap((k) => [`documents.generate.kind.${k}`, `documents.generate.kindHelp.${k}`]),
      ...DOC_GROUPINGS.map((g) => `documents.generate.group.${g}`),
      ...LANGUAGES.map((l) => `documents.generate.lang.${l}`),
      ...problems.map((p) => `documents.generate.problem.${p}`),
    ]
    expect(problems.length).toBe(3)
    expect(keys.filter((k) => !(k in EN))).toEqual([])
  })
  it('cover all analysis rules', async () => {
    const { ANALYSIS_RULES } = await import('./analysis')
    expect(ANALYSIS_RULES.filter((r) => !(`analysis.rules.${r}` in EN))).toEqual([])
  })
})
