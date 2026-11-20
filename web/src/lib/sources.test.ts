import { describe, expect, it } from 'vitest'
import { filterSources, mergeSources } from './sources'

describe('requirement sources', () => {
  it('merges configured and used sources', () => {
    expect(mergeSources(['Customer', 'Standard'], ['customer', 'RFP v1.2', ' ', 'Standard '])).toEqual(['Customer', 'Standard', 'RFP v1.2'])
  })
  it('filters by prefix, then substring', () => {
    const all = ['Customer', 'Contract', 'Security analysis', 'Customs']
    expect(filterSources(all, 'cu')).toEqual(['Customer', 'Customs', 'Security analysis'])
    expect(filterSources(all, 'ana')).toEqual(['Security analysis'])
    expect(filterSources(all, 'r')).toEqual(['Customer', 'Contract', 'Security analysis'])
    expect(filterSources(all, '  ')).toEqual(all)
  })
})
