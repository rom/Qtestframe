// Which guide page explains which app page (Help → "Help for this page",
// and the "?" links on the main pages).
import { helpPath } from './links'

const BY_ROUTE: Record<string, string> = {
  home: 'getting-started',
  projects: 'tutorial',
  dashboard: 'traceability-and-coverage',
  requirements: 'requirements',
  tests: 'tests',
  runs: 'runs',
  run: 'runs',
  execute: 'runs',
  cycles: 'test-cycles',
  cycle: 'test-cycles',
  traceability: 'traceability-and-coverage',
  graph: 'traceability-and-coverage',
  catalog: 'catalogue',
  tags: 'search-and-filters',
  baselines: 'baselines',
  settings: 'tests',
  export: 'reports-and-export',
  reports: 'reports-and-export',
  waivers: 'waivers-and-soa',
  soa: 'waivers-and-soa',
  documents: 'documents',
  search: 'search-and-filters',
  profile: 'getting-started',
  admin: 'administration',
  members: 'administration',
  tokens: 'administration',
  audit: 'administration',
  tenants: 'administration',
  users: 'administration',
  backup: 'administration',
}

/** The guide page for a route name, or null (e.g. on the guide itself). */
export function guidePageForRoute(name: unknown): string | null {
  if (typeof name !== 'string') return null
  if (BY_ROUTE[name]) return BY_ROUTE[name]
  if (name.startsWith('admin')) return 'administration'
  return null
}

/** Router path of the guide page for a route name (the guide index when there is none). */
export function contextHelpPath(name: unknown): string {
  return helpPath(guidePageForRoute(name) ?? 'index')
}
