import { describe, expect, it } from 'vitest'
import { LINK_ICONS, LINK_TYPES, linkTarget } from './itemLinks'
import type { ItemLink } from './types'

const link = (o: Partial<ItemLink>): ItemLink => ({
  id: 'l', project_id: 'p', source_type: 'test', source_id: 's', target_type: 'requirement', target_id: 't', url: '', label: '',
  created_by: '', created_at: '', source_key: 'TC-1', source_title: '', target_key: 'REQ-1', target_title: '', ...o,
})

describe('item links', () => {
  it('leads to items, files and addresses', () => {
    expect(linkTarget(link({}), 'x').to).toEqual({ name: 'requirements', params: { projectId: 'p' }, query: { item: 't' } })
    expect(linkTarget(link({ target_type: 'test' }), 'x').to).toEqual({ name: 'tests', params: { projectId: 'p' }, query: { item: 't' } })
    expect(linkTarget(link({ target_type: 'run' }), 'x').to).toEqual({ name: 'run', params: { projectId: 'p', runId: 't' } })
    expect(linkTarget(link({ target_type: 'report' }), 'x').to).toEqual({ name: 'reports', params: { projectId: 'p' }, query: { definition: 't' } })
    expect(linkTarget(link({ target_type: 'file' }), 'x').href).toContain('/files/t/content')
    expect(linkTarget(link({ target_type: 'url', target_id: null, url: 'https://x.org' }), 'x')).toEqual({ href: 'https://x.org', external: true })
    expect(linkTarget(link({ target_id: null }), 'x')).toEqual({})
    expect(linkTarget(link({ target_type: 'other' }), 'x')).toEqual({})
  })
  it('leads back to the item that links here', () => {
    expect(linkTarget(link({ project_id: '' }), 'proj', true).to).toEqual({ name: 'tests', params: { projectId: 'proj' }, query: { item: 's' } })
  })
  it('has an icon per type', () => {
    for (const t of LINK_TYPES) expect(LINK_ICONS[t]).toMatch(/^pi pi-/)
  })
})
