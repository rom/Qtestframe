import { describe, expect, it } from 'vitest'
import {
  canRelease,
  changeCriterionType,
  cleanCriteria,
  criteriaCounts,
  criterionParam,
  criterionText,
  newCriterion,
  percent,
  releaseBaselineName,
  releaseItemRoute,
  resultValues,
  runReleaseName,
  runsOfRelease,
  CRITERION_TYPES,
} from './releases'
import type { CriterionResult } from './types'

const t = (key: string, params?: Record<string, unknown>) => (params ? `${key} ${JSON.stringify(params)}` : key)
const level = (kind: string, v: string) => `${kind}:${v}`
const result = (p: Partial<CriterionResult>): CriterionResult => ({ type: 'coverage', met: true, actual: 0, total: 0, items: [], available: true, ...p })

describe('trace rules criterion', () => {
  it('cleans the level and links rule items to the rules', () => {
    expect(cleanCriteria([{ type: 'trace_rules', level: 'warning', threshold: 5 }, { type: 'trace_rules', level: 'x' }, { type: 'trace_rules' }])).toEqual([
      { type: 'trace_rules', level: 'warning' },
      { type: 'trace_rules', level: 'error' },
      { type: 'trace_rules', level: 'error' },
    ])
    expect(releaseItemRoute({ kind: 'trace_rule', id: 'r1' }, 'p1')).toEqual({ name: 'traceability', params: { projectId: 'p1' }, query: { view: 'rules' } })
  })
})

describe('criterion parameters', () => {
  it('knows what each type takes', () => {
    expect(CRITERION_TYPES.map(criterionParam)).toEqual(['threshold', 'threshold', 'none', 'threshold', 'none', 'threshold', 'none', 'severity', 'riskLevel', 'ruleSeverity'])
  })
  it('creates criteria with defaults', () => {
    expect(newCriterion('passed')).toEqual({ type: 'passed', threshold: 100 })
    expect(newCriterion('no_open_defects')).toEqual({ type: 'no_open_defects', level: 'major' })
    expect(newCriterion('risks_mitigated')).toEqual({ type: 'risks_mitigated', level: 'high' })
    expect(newCriterion('no_failed')).toEqual({ type: 'no_failed' })
    expect(newCriterion('trace_rules')).toEqual({ type: 'trace_rules', level: 'error' })
  })
  it('keeps the threshold when the type stays a percentage', () => {
    expect(changeCriterionType({ type: 'coverage', threshold: 80 }, 'passed')).toEqual({ type: 'passed', threshold: 80 })
    expect(changeCriterionType({ type: 'coverage', threshold: 80 }, 'no_suspect')).toEqual({ type: 'no_suspect' })
    expect(changeCriterionType({ type: 'no_failed' }, 'executed')).toEqual({ type: 'executed', threshold: 100 })
  })
  it('cleans criteria for the API', () => {
    expect(
      cleanCriteria([
        { type: 'coverage', threshold: 250, level: 'x' },
        { type: 'passed', threshold: Number.NaN },
        { type: 'executed', threshold: 0 },
        { type: 'no_open_defects', level: '', threshold: 3 },
        { type: 'risks_mitigated', level: 'critical' },
        { type: 'no_failed', threshold: 5 },
      ]),
    ).toEqual([
      { type: 'coverage', threshold: 100 },
      { type: 'passed', threshold: 100 },
      { type: 'executed', threshold: 100 },
      { type: 'no_open_defects' },
      { type: 'risks_mitigated', level: 'critical' },
      { type: 'no_failed' },
    ])
  })
})

describe('criterion text and values', () => {
  it('describes criteria', () => {
    expect(percent(92.54)).toBe('92.5 %')
    expect(percent(undefined)).toBe('0 %')
    expect(criterionText({ type: 'coverage', threshold: 95 }, t, level)).toBe('releasesView.criterion.coverage.withThreshold {"threshold":"95 %"}')
    expect(criterionText({ type: 'passed' }, t, level)).toContain('100 %')
    expect(criterionText({ type: 'no_open_defects', level: 'major' }, t, level)).toBe('releasesView.criterion.no_open_defects.withLevel {"level":"severity:major"}')
    expect(criterionText({ type: 'risks_mitigated', level: 'high' }, t, level)).toContain('riskLevel:high')
    expect(criterionText({ type: 'risks_mitigated' }, t, level)).toBe('releasesView.criterion.risks_mitigated.any')
    expect(criterionText({ type: 'no_failed' }, t, level)).toBe('releasesView.criterion.no_failed.label')
    expect(criterionText({ type: 'trace_rules', level: 'error' }, t, level)).toBe('releasesView.criterion.trace_rules.errorsOnly')
    expect(criterionText({ type: 'trace_rules', level: 'warning' }, t, level)).toBe('releasesView.criterion.trace_rules.withWarnings')
  })
  it('gives actual and target', () => {
    expect(resultValues(result({ type: 'passed', actual: 80, threshold: 95 }))).toEqual({ actual: '80 %', target: '≥ 95 %' })
    expect(resultValues(result({ type: 'coverage', actual: 100 }))).toEqual({ actual: '100 %', target: '≥ 100 %' })
    expect(resultValues(result({ type: 'no_failed', actual: 3 }))).toEqual({ actual: '3', target: '0' })
    expect(criteriaCounts([result({}), result({ met: false })])).toEqual({ met: 1, unmet: 1 })
    expect(criteriaCounts(null)).toEqual({ met: 0, unmet: 0 })
  })
})

describe('links and runs', () => {
  it('routes items by kind', () => {
    expect(releaseItemRoute({ kind: 'requirement', id: '1' }, 'p')).toEqual({ name: 'requirements', params: { projectId: 'p' }, query: { item: '1' } })
    expect(releaseItemRoute({ kind: 'test', id: '2' }, 'p')).toMatchObject({ name: 'tests', query: { item: '2' } })
    expect(releaseItemRoute({ kind: 'risk', id: '3' }, 'p')).toMatchObject({ name: 'risks', query: { risk: '3' } })
    expect(releaseItemRoute({ kind: 'defect', id: '4' }, 'p')).toMatchObject({ name: 'defects', query: { defect: '4' } })
  })
  it('finds the runs and names of releases', () => {
    const runs = [{ release_id: 'a' }, { release_id: null }, { release_id: 'b' }]
    expect(runsOfRelease(runs, 'a')).toEqual([{ release_id: 'a' }])
    const names = new Map([['a', 'Spring']])
    expect(runReleaseName({ release_id: 'a' }, names)).toBe('Spring')
    expect(runReleaseName({ release_id: 'a', release_name: 'Given' }, names)).toBe('Given')
    expect(runReleaseName({ release_id: 'zz' }, names)).toBe('')
    expect(runReleaseName({ release_id: null }, names)).toBe('')
  })
  it('names the release baseline and knows when releasing is possible', () => {
    expect(releaseBaselineName({ name: '2.0' })).toBe('Release 2.0')
    expect(releaseBaselineName({ name: '2.0', version: '2.0' })).toBe('Release 2.0')
    expect(releaseBaselineName({ name: 'Spring', version: '2.1' })).toBe('Release Spring (2.1)')
    expect(canRelease('active')).toBe(true)
    expect(canRelease('planned')).toBe(true)
    expect(canRelease('released')).toBe(false)
  })
})
