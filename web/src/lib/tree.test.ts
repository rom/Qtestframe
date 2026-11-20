import { describe, expect, it } from 'vitest'
import { allKeys, ancestorKeys, buildTree, canMoveFolder, checkedIds, descendantIds, flatten, folderPath, topMostFolders } from './tree'
import type { Folder } from './types'

function f(id: string, parent: string | null, name = id, position = 0): Folder {
  return { id, parent_id: parent, name, position, project_id: 'p', kind: 'requirement', description: '', created_at: '' }
}

const folders = [f('a', null, 'Alpha', 2), f('b', null, 'Beta', 1), f('a1', 'a', 'One'), f('a2', 'a', 'Two'), f('a1x', 'a1', 'Deep')]

describe('buildTree', () => {
  it('handles empty input', () => {
    expect(buildTree(null)).toEqual([])
    expect(buildTree(undefined)).toEqual([])
  })
  it('nests by parent_id and sorts by position then name', () => {
    const t = buildTree(folders)
    expect(t.map((n) => n.key)).toEqual(['b', 'a'])
    expect(t[1].children.map((n) => n.label)).toEqual(['One', 'Two'])
    expect(t[1].children[0].children[0].key).toBe('a1x')
    expect(t[1].leaf).toBe(false)
    expect(t[0].leaf).toBe(true)
    expect(t[0].data.name).toBe('Beta')
  })
  it('attaches orphans and cycles to the root', () => {
    const t = buildTree([f('x', 'missing'), f('c1', 'c2'), f('c2', 'c1')])
    expect(t.map((n) => n.key).sort()).toEqual(['c1', 'c2', 'x'])
  })
})

describe('descendants and moves', () => {
  it('collects descendants including self', () => {
    expect([...descendantIds(folders, 'a')].sort()).toEqual(['a', 'a1', 'a1x', 'a2'])
    expect([...descendantIds(folders, 'b')]).toEqual(['b'])
    expect([...descendantIds([f('c1', 'c2'), f('c2', 'c1')], 'c1')].sort()).toEqual(['c1', 'c2'])
  })
  it('rejects cycles and no-op moves', () => {
    expect(canMoveFolder(folders, 'a', 'a1x')).toBe(false)
    expect(canMoveFolder(folders, 'a', 'a')).toBe(false)
    expect(canMoveFolder(folders, 'a1', 'a')).toBe(false)
    expect(canMoveFolder(folders, 'a', null)).toBe(false)
    expect(canMoveFolder(folders, 'a1', null)).toBe(true)
    expect(canMoveFolder(folders, 'a1', 'b')).toBe(true)
    expect(canMoveFolder(folders, 'zzz', 'b')).toBe(false)
  })
})

describe('paths', () => {
  it('folderPath', () => {
    expect(folderPath(folders, 'a1x')).toBe('Alpha / One / Deep')
    expect(folderPath(folders, 'a1x', '/')).toBe('Alpha/One/Deep')
    expect(folderPath(folders, null)).toBe('')
    expect(folderPath(folders, 'nope')).toBe('')
    expect(folderPath([f('c1', 'c2'), f('c2', 'c1')], 'c1')).toBe('c1 / c2'.split(' / ').reverse().join(' / '))
  })
  it('ancestorKeys', () => {
    expect(ancestorKeys(folders, 'a1x')).toEqual(['a', 'a1'])
    expect(ancestorKeys(folders, 'a')).toEqual([])
    expect(ancestorKeys([f('c1', 'c2'), f('c2', 'c1')], 'c1').length).toBeLessThanOrEqual(2)
  })
  it('allKeys and flatten', () => {
    const t = buildTree(folders)
    expect(Object.keys(allKeys(t)).sort()).toEqual(['a', 'a1', 'a1x', 'a2', 'b'])
    expect(flatten(t)).toEqual([
      { id: 'b', label: 'Beta', depth: 0 },
      { id: 'a', label: 'Alpha', depth: 0 },
      { id: 'a1', label: 'One', depth: 1 },
      { id: 'a1x', label: 'Deep', depth: 2 },
      { id: 'a2', label: 'Two', depth: 1 },
    ])
  })
})

describe('bulk selection', () => {
  const f = (id: string, parent: string | null) => ({ id, parent_id: parent, name: id }) as unknown as Folder
  const folders = [f('a', null), f('a1', 'a'), f('a11', 'a1'), f('b', null)]
  it('keeps the top-most folders', () => {
    expect(topMostFolders(folders, ['a1', 'a', 'a11', 'b'])).toEqual(['a', 'b'])
    expect(topMostFolders(folders, ['a11', 'b'])).toEqual(['a11', 'b'])
  })
  it('reads checked keys', () => {
    expect(checkedIds({ a: { checked: true, partialChecked: false }, b: { checked: false, partialChecked: true }, c: true, d: undefined })).toEqual(['a', 'c'])
  })
})
