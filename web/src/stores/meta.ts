import { defineStore } from 'pinia'
import { api } from '@/lib/api'
import type { Meta } from '@/lib/types'

/** Defaults mirror internal/model so the UI works before /meta answers. */
export const DEFAULT_META: Meta = {
  obligations: ['must', 'must_not', 'shall', 'shall_not', 'should', 'should_not', 'may', 'optional', 'will', 'describe'],
  requirement_statuses: ['draft', 'review', 'approved', 'implemented', 'verified', 'rejected', 'obsolete'],
  requirement_types: ['functional', 'non_functional', 'security', 'performance', 'usability', 'interface', 'constraint', 'business', 'regulatory'],
  priorities: ['critical', 'high', 'medium', 'low'],
  test_statuses: ['draft', 'ready', 'review', 'deprecated'],
  script_languages: ['sh', 'bash', 'python', 'javascript', 'playwright'],
  evaluation_methods: ['test', 'inspection', 'analysis', 'demonstration', 'review'],
  run_statuses: ['planned', 'in_progress', 'completed', 'aborted'],
  result_statuses: ['not_run', 'in_progress', 'passed', 'failed', 'blocked', 'skipped'],
  roles: ['admin', 'manager', 'tester', 'viewer'],
  analysis_languages: ['en', 'sv'],
  auto_create_test: true,
  email_enabled: false,
  runner_enabled: false,
  web_tests_enabled: false,
}

export const useMetaStore = defineStore('meta', {
  state: () => ({ meta: { ...DEFAULT_META } as Meta, loaded: false }),
  actions: {
    async load() {
      if (this.loaded) return
      try {
        const m = await api.get<Partial<Meta>>('/meta')
        const merged = { ...DEFAULT_META } as Meta
        for (const k of Object.keys(DEFAULT_META) as (keyof Meta)[]) {
          const v = m?.[k]
          if (Array.isArray(v) && v.length) (merged as unknown as Record<string, unknown>)[k] = v
        }
        if (typeof m?.auto_create_test === 'boolean') merged.auto_create_test = m.auto_create_test
        merged.email_enabled = m?.email_enabled === true
        merged.runner_enabled = m?.runner_enabled === true
        merged.web_tests_enabled = m?.web_tests_enabled === true
        if (typeof m?.email_digest_hour === 'number') merged.email_digest_hour = m.email_digest_hour
        this.meta = merged
        this.loaded = true
      } catch {
        /* keep defaults */
      }
    },
  },
})
