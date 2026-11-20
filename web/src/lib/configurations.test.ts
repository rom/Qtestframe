import { describe, expect, it } from 'vitest'
import {
  combinations,
  configurationName,
  coverageInConfiguration,
  coverageOf,
  dimensionsOf,
  existingNames,
  overallCoverage,
  resultIn,
  splitValues,
  summaryByConfiguration,
  type CoverageTest,
} from './configurations'
import type { Configuration, Execution } from './types'

const cfg = (name: string, values: [string, string][]): Configuration => ({
  id: name,
  project_id: 'p',
  name,
  description: '',
  position: 0,
  values: values.map(([dimension, value]) => ({ dimension, value })),
})

describe('configurations', () => {
  it('splits values', () => {
    expect(splitValues(' Chrome, firefox\nChrome;  Safari  Tech ,')).toEqual(['Chrome', 'firefox', 'Safari Tech'])
  })
  it('names a configuration after its values', () => {
    expect(configurationName([{ dimension: 'B', value: ' Chrome ' }, { dimension: 'OS', value: '' }, { dimension: 'X', value: 'Win' }])).toBe('Chrome / Win')
  })
  it('builds every combination', () => {
    expect(combinations([])).toEqual([])
    expect(combinations([{ dimension: ' ', values: ['x'] }, { dimension: 'E', values: [] }])).toEqual([])
    const c = combinations([
      { dimension: 'Browser', values: ['Chrome', 'Firefox'] },
      { dimension: 'OS', values: ['Windows', ' ', 'macOS'] },
    ])
    expect(c.map(configurationName)).toEqual(['Chrome / Windows', 'Chrome / macOS', 'Firefox / Windows', 'Firefox / macOS'])
    expect(c[0]).toEqual([
      { dimension: 'Browser', value: 'Chrome' },
      { dimension: 'OS', value: 'Windows' },
    ])
  })
  it('collects dimensions and existing names', () => {
    const list = [cfg('Chrome / Windows', [['Browser', 'Chrome'], ['OS', 'Windows']]), cfg('Firefox', [['browser', 'Firefox'], ['Browser', 'Chrome']]), { ...cfg('x', []), values: undefined as never }]
    expect(dimensionsOf(list)).toEqual([
      { dimension: 'Browser', values: ['Chrome', 'Firefox'] },
      { dimension: 'OS', values: ['Windows'] },
    ])
    expect(existingNames(list, ['firefox', 'Safari'])).toEqual(['firefox'])
  })
  it('summarises executions per configuration', () => {
    const ex = (status: string, id?: string, name?: string) => ({ status, configuration_id: id ?? null, configuration_name: name }) as Execution
    const s = summaryByConfiguration([ex('passed', 'c', 'Chrome'), ex('failed', 'f', 'Firefox'), ex('passed', 'c', 'Chrome'), ex('not_run')], [
      { id: 'f', name: 'Firefox' },
      { id: 's', name: 'Safari' },
    ])
    expect(s).toEqual([
      { id: 'f', name: 'Firefox', summary: { total: 1, failed: 1 } },
      { id: 's', name: 'Safari', summary: { total: 0 } },
      { id: 'c', name: 'Chrome', summary: { total: 2, passed: 2 } },
      { id: '', name: '', summary: { total: 1, not_run: 1 } },
    ])
  })
})

describe('coverage per configuration', () => {
  it('rates results', () => {
    expect(coverageOf([])).toBe('none')
    expect(coverageOf(['passed', 'blocked'])).toBe('failed')
    expect(coverageOf(['passed', 'passed'])).toBe('passed')
    expect(coverageOf(['passed', ''])).toBe('partial')
    expect(coverageOf([null, 'skipped'])).toBe('not_run')
  })
  const t = (key: string, results: Record<string, string>): CoverageTest => ({ id: key, key, title: key, results })
  const tests = [t('a', { c: 'passed', f: 'passed' }), t('b', { c: 'passed', f: '' }), t('d', { '': 'passed' })]
  it('counts every planned configuration overall', () => {
    expect(overallCoverage([])).toBe('none')
    expect(overallCoverage([tests[0]])).toBe('passed')
    expect(overallCoverage(tests)).toBe('partial')
    expect(overallCoverage([t('x', {})])).toBe('not_run')
    expect(overallCoverage([{ ...t('y', {}), results: undefined as never }])).toBe('not_run')
  })
  it('rates one configuration over the tests planned in it', () => {
    expect(coverageInConfiguration([], 'c')).toBe('none')
    expect(coverageInConfiguration(tests, 'c')).toBe('passed')
    expect(coverageInConfiguration(tests, 'f')).toBe('partial')
    expect(coverageInConfiguration(tests, '')).toBe('passed')
    expect(coverageInConfiguration(tests, 'zz')).toBe('not_run')
    expect(coverageInConfiguration([{ ...t('y', {}), results: undefined as never }], 'c')).toBe('not_run')
  })
  it('tells the result of a test in a configuration', () => {
    expect(resultIn(tests[0], 'c')).toBe('passed')
    expect(resultIn(tests[1], 'f')).toBe('not_run')
    expect(resultIn(tests[1], 'x')).toBe('')
    expect(resultIn({ ...tests[0], results: undefined as never }, 'c')).toBe('')
  })
})
