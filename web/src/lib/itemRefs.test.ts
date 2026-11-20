import { describe, expect, it } from 'vitest'
import { groupRefs, refKind, refsText } from './itemRefs'

const ref = (type: string, direction: string, key: string, id = key) => ({ type, direction, key, id })

describe('item references', () => {
  it('knows what is at the other end', () => {
    expect(refKind(ref('depends_on', 'incoming', 'R'))).toBe('requirement')
    expect(refKind(ref('verifies', 'outgoing', 'R'))).toBe('requirement')
    expect(refKind(ref('verifies', 'incoming', 'T'))).toBe('test')
    expect(refKind(ref('calls', 'incoming', 'T'))).toBe('test')
    expect(refKind({ ...ref('depends_on', 'outgoing', 'T'), kind: 'test' })).toBe('test')
  })
  it('groups by type and direction in a stable order, without duplicates', () => {
    const g = groupRefs([
      ref('verifies', 'incoming', 'TC-1'),
      ref('provides', 'incoming', 'REQ-3'),
      ref('depends_on', 'outgoing', 'REQ-2'),
      ref('depends_on', 'incoming', 'REQ-9'),
      ref('depends_on', 'outgoing', 'REQ-4'),
      ref('depends_on', 'outgoing', 'REQ-4'),
      ref('custom', 'outgoing', 'X'),
    ])
    expect(g.map((x) => `${x.type}/${x.direction}:${x.items.map((i) => i.key).join('+')}`)).toEqual([
      'depends_on/outgoing:REQ-2+REQ-4',
      'depends_on/incoming:REQ-9',
      'provides/incoming:REQ-3',
      'verifies/incoming:TC-1',
      'custom/outgoing:X',
    ])
    expect(groupRefs(null)).toEqual([])
    expect(refsText(g.slice(0, 2), (t, d) => `${t}-${d}`)).toBe('depends_on-outgoing REQ-2, REQ-4; depends_on-incoming REQ-9')
  })
})
