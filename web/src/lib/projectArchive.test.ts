import { describe, expect, it } from 'vitest'
import { archiveSummary, freeProjectKey, looksLikeZip, normalizeProjectKey, totalRows, validProjectKey } from './projectArchive'

describe('project keys', () => {
  it('normalizes', () => {
    expect(normalizeProjectKey(' de-mo 1 ')).toBe('DEMO1')
    expect(normalizeProjectKey('a'.repeat(20))).toBe('A'.repeat(16))
  })
  it('validates', () => {
    expect(validProjectKey('demo')).toBe(true)
    expect(validProjectKey('D')).toBe(false)
    expect(validProjectKey('1AB')).toBe(false)
    expect(validProjectKey('')).toBe(false)
  })
  it('finds a free key', () => {
    expect(freeProjectKey('new', ['DEMO'])).toBe('NEW')
    expect(freeProjectKey('demo', ['DEMO'])).toBe('DEMO2')
    expect(freeProjectKey('DEMO', ['demo', 'DEMO2', 'DEMO3'])).toBe('DEMO4')
    expect(freeProjectKey('ABCDEFGHIJKLMNOP', ['ABCDEFGHIJKLMNOP'])).toBe('ABCDEFGHIJKLMNO2')
    expect(freeProjectKey('', [])).toBe('P2')
    expect(freeProjectKey('X', [])).toBe('X2')
  })
})

describe('archive summary', () => {
  it('counts', () => {
    const s = archiveSummary({ requirements: 3, test_cases: 2, test_runs: 1, test_executions: 4, folders: 2 }, 5)
    expect(s).toEqual({ requirements: 3, tests: 2, runs: 1, executions: 4, defects: 0, risks: 0, baselines: 0, files: 5, rows: 12 })
    expect(archiveSummary(undefined).rows).toBe(0)
    expect(totalRows({ a: 1, b: Number.NaN })).toBe(1)
  })
  it('recognises ZIP files', () => {
    expect(looksLikeZip({ name: 'demo.ZIP' })).toBe(true)
    expect(looksLikeZip({ name: 'x', type: 'application/zip' })).toBe(true)
    expect(looksLikeZip({ name: 'x.txt', type: 'text/plain' })).toBe(false)
  })
})
