// Kinds of project (mirrors model.ProjectTypes). Software projects get extra
// features: prompts for AI assistants from requirements and tests.
import type { Project } from './types'

export const PROJECT_TYPES = ['general', 'software', 'hardware', 'system', 'business'] as const
export type ProjectType = (typeof PROJECT_TYPES)[number]

export const PROJECT_TYPE_ICONS: Record<ProjectType, string> = {
  general: 'pi pi-folder',
  software: 'pi pi-code',
  hardware: 'pi pi-microchip',
  system: 'pi pi-sitemap',
  business: 'pi pi-briefcase',
}

export const isSoftware = (p: Pick<Project, 'type'> | null | undefined): boolean => p?.type === 'software'

/** Prompt kinds (mirrors prompt.Kinds); the first three take requirements. */
export const PROMPT_KINDS = ['implement', 'review_requirements', 'design_tests', 'automate_tests', 'review_tests', 'test_strategy'] as const
export type PromptKind = (typeof PROMPT_KINDS)[number]
export const promptForRequirements = (k: string): boolean => k === 'implement' || k === 'review_requirements' || k === 'design_tests'
export const promptNeedsTestType = (k: string): boolean => k === 'design_tests' || k === 'test_strategy'
export const isPromptKind = (k: unknown): k is PromptKind => typeof k === 'string' && (PROMPT_KINDS as readonly string[]).includes(k)

/** Test types (mirrors prompt.TestTypes). */
export const TEST_TYPES = ['unit', 'integration', 'system', 'acceptance', 'end_to_end', 'api', 'regression', 'smoke', 'performance', 'security', 'usability', 'accessibility'] as const

/** Frameworks offered as suggestions (free text is fine). */
export const FRAMEWORK_SUGGESTIONS = [
  'Go testing',
  'pytest',
  'unittest (Python)',
  'JUnit 5',
  'TestNG',
  'NUnit',
  'xUnit.net',
  'Jest',
  'Vitest',
  'Mocha',
  'Playwright',
  'Cypress',
  'Selenium WebDriver',
  'Robot Framework',
  'Cucumber',
  'RSpec',
  'PHPUnit',
  'Catch2',
  'GoogleTest',
  'k6',
  'JMeter',
  'Gatling',
  'OWASP ZAP',
  'axe-core',
  'Postman / Newman',
  'REST Assured',
  'Bats',
]

/** File name for a downloaded prompt: "SHOP-implement-2026-11-20.md". */
export function promptFileName(projectKey: string, kind: string, date = new Date()): string {
  const d = date.toISOString().slice(0, 10)
  return `${projectKey || 'prompt'}-${kind.replace(/_/g, '-')}-${d}.md`
}
