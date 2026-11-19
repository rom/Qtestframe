import { describe, expect, it } from 'vitest'
import { decodeIds, dragIds, encodeIds, FOLDER_MIME, ITEMS_MIME } from './dnd'

describe('dnd payloads', () => {
  it('round-trips ids', () => {
    expect(decodeIds(encodeIds(['a', 'b']))).toEqual(['a', 'b'])
    expect(ITEMS_MIME).not.toBe(FOLDER_MIME)
  })
  it('rejects malformed payloads', () => {
    expect(decodeIds('')).toEqual([])
    expect(decodeIds(null)).toEqual([])
    expect(decodeIds('{')).toEqual([])
    expect(decodeIds('{"a":1}')).toEqual([])
    expect(decodeIds('["a", 1, "", null]')).toEqual(['a'])
  })
  it('drags the selection when the row is selected', () => {
    expect(dragIds('a', [{ id: 'a' }, { id: 'b' }])).toEqual(['a', 'b'])
    expect(dragIds('c', [{ id: 'a' }])).toEqual(['c'])
    expect(dragIds('c', null)).toEqual(['c'])
  })
})
