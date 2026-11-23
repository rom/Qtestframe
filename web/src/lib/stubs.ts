// Generated test code stubs (GET/POST /projects/{pid}/tests/stubs, see
// docs/api.md "Test stubs" and internal/stubgen).

export const STUB_FRAMEWORKS = [
  { id: 'pytest', name: 'pytest', package: '' },
  { id: 'playwright', name: 'Playwright (TypeScript)', package: '' },
  { id: 'go', name: 'Go testing', package: 'go' },
  { id: 'junit5', name: 'JUnit 5 (Java)', package: 'java' },
  { id: 'jest', name: 'Jest (TypeScript)', package: '' },
  { id: 'vitest', name: 'Vitest (TypeScript)', package: '' },
  { id: 'robot', name: 'Robot Framework', package: '' },
  { id: 'cucumber', name: 'Cucumber (Gherkin)', package: '' },
] as const

export type StubFrameworkId = (typeof STUB_FRAMEWORKS)[number]['id']

export interface StubFile {
  path: string
  content: string
  key: string
  framework: string
  test_id: string
}

export interface StubResponse {
  framework: { id: string; name: string; language: string; runner: string; script_language?: string; parametrised: boolean }
  files: StubFile[]
  marked: number
}

export interface StubRequest {
  framework: StubFrameworkId
  query?: string
  ids?: string[]
  format: 'json' | 'zip' | 'file'
  package?: string
  dir?: string
  mark_script?: boolean
}

/** Quote a value for a POSIX shell (single quotes). */
export function shellQuote(s: string): string {
  if (/^[\w@%+=:,./-]+$/.test(s)) return s
  return `'${s.replace(/'/g, `'\\''`)}'`
}

/** The qtestframe stubs command that writes the same stubs into a directory. */
export function stubsCommand(o: { framework: string; query?: string; keys?: string[]; dir?: string; pkg?: string; markScript?: boolean }): string {
  const parts = ['qtestframe stubs', '--framework', o.framework]
  if (o.keys?.length) parts.push('--query', "''", '--ids', o.keys.join(','))
  else parts.push('--query', shellQuote(o.query ?? ''))
  parts.push('--out', shellQuote(o.dir?.trim() || '.'))
  if (o.pkg?.trim()) parts.push('--package', shellQuote(o.pkg.trim()))
  if (o.markScript) parts.push('--mark-script')
  return parts.join(' ')
}
